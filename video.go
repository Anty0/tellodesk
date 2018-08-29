package main

import (
	"bufio"
	"image"
	"log"
	"os"
	"time"

	"github.com/3d0c/gmf"
)

// func (app *tdApp) startVideoCB(s string, i interface{}) {

// 	var err error

// 	app.videoChan, err = drone.VideoConnectDefault()
// 	if err != nil {
// 		alertDialog(app, errorSev, err.Error())
// 	}

// 	// start video feed when drone connects
// 	drone.StartVideo()
// 	go func() {
// 		for {
// 			drone.StartVideo()
// 			time.Sleep(500 * time.Millisecond)
// 		}
// 	}()

// 	app.videoStopChan = make(chan bool) // unbuffered

// 	go app.videoListener()
// }

func (app *tdApp) recordVideoCB(s string, i interface{}) {
	var vidPath string
	cwd, _ := os.Getwd()
	fs, _ := NewFileSelect(app.mainPanel, cwd, "Choose File for Video Recording", "*.h264")
	fs.Subscribe("OnOK", func(n string, ev interface{}) {
		vidPath = fs.Selected()
		//app.Log().Info("Selected: %s", vidPath)
		if vidPath != "" {
			var err error
			app.videoFile, err = os.OpenFile(vidPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
			if err != nil {
				alertDialog(app.mainPanel, errorSev, "Could not open video file")
			} else {
				app.videoWriter = bufio.NewWriter(app.videoFile)
				app.videoRecMu.Lock()
				app.videoRecording = true
				app.videoRecMu.Unlock()
				app.recordVideoItem.SetEnabled(false)
				app.stopRecordingItem.SetEnabled(true)
			}
		}
		fs.Close()
	})
	fs.Subscribe("OnCancel", func(n string, ev interface{}) {
		fs.Close()
	})
}

func (app *tdApp) stopRecordingCB(s string, i interface{}) {
	app.videoRecMu.Lock()
	app.videoRecording = false
	app.videoRecMu.Unlock()
	app.videoWriter.Flush()
	app.videoFile.Close()
	app.recordVideoItem.SetEnabled(true)
	app.stopRecordingItem.SetEnabled(false)
}

func (app *tdApp) startVideo() {

	var err error

	app.videoChan, err = drone.VideoConnectDefault()
	if err != nil {
		alertDialog(app.mainPanel, errorSev, err.Error())
	}

	// start video feed when drone connects
	drone.StartVideo()
	go func() {
		for {
			drone.StartVideo()
			time.Sleep(500 * time.Millisecond)
		}
	}()

	//app.videoStopChan = make(chan bool) // unbuffered

	go videoListener(app)
}

func (app *tdApp) customReader() ([]byte, int) {
	pkt := <-app.videoChan
	app.videoRecMu.RLock()
	if app.videoRecording {
		app.videoWriter.Write(pkt)
	}
	app.videoRecMu.RUnlock()
	return pkt, len(pkt)
}

func assert(i interface{}, err error) interface{} {
	if err != nil {
		log.Fatalf("Assert %v", err)
	}

	return i
}

//func (app *tdApp) videoListener() {
func videoListener(app *tdApp) {

	//app.Log().Info("Videolistener started")

	iCtx := gmf.NewCtx()
	defer iCtx.CloseInputAndRelease()

	if err := iCtx.SetInputFormat("h264"); err != nil {
		log.Fatalf("iCtx SetInputFormat %v", err)
	}
	//app.Log().Info("Input format set")
	avioCtx, err := gmf.NewAVIOContext(iCtx, &gmf.AVIOHandlers{ReadPacket: app.customReader})
	defer gmf.Release(avioCtx)
	if err != nil {
		log.Fatalf("NewAVIOContext %v", err)
	}

	//app.Log().Info("Setting Pb...")
	iCtx.SetPb(avioCtx)

	//app.Log().Info("Opening input...")
	err = iCtx.OpenInput("")
	if err != nil {
		log.Fatalf("iCtx OpenInput %v", err)
	}

	//app.Log().Info("Getting best stream...")
	srcVideoStream, err := iCtx.GetBestStream(gmf.AVMEDIA_TYPE_VIDEO)
	if err != nil {
		log.Fatalf("GetBestStream %v", err)
	}

	// codec, err := gmf.FindEncoder(gmf.AV_CODEC_ID_PNG)
	codec, err := gmf.FindEncoder(gmf.AV_CODEC_ID_RAWVIDEO)
	if err != nil {
		log.Fatalf("FindDecoder %v", err)
	}
	cc := gmf.NewCodecCtx(codec)
	defer gmf.Release(cc)

	if codec.IsExperimental() {
		cc.SetStrictCompliance(gmf.FF_COMPLIANCE_EXPERIMENTAL)
	}

	// cc.SetPixFmt(gmf.AV_PIX_FMT_RGB24).
	cc.SetPixFmt(gmf.AV_PIX_FMT_BGR32).
		SetWidth(videoWidth).
		SetHeight(videoHeight).
		SetTimeBase(gmf.AVR{Num: 1, Den: 1})
	//app.Log().Info("Opening cc")
	if err := cc.Open(nil); err != nil {
		log.Fatalf("cc Open %v", err)
	}

	swsCtx := gmf.NewSwsCtx(srcVideoStream.CodecCtx(), cc, gmf.SWS_BICUBIC)
	defer gmf.Release(swsCtx)

	dstFrame := gmf.NewFrame().
		SetWidth(videoWidth).
		SetHeight(videoHeight).
		SetFormat(gmf.AV_PIX_FMT_BGR32) //SetFormat(gmf.AV_PIX_FMT_RGB32)
	defer gmf.Release(dstFrame)

	if err := dstFrame.ImgAlloc(); err != nil {
		log.Fatalf("ImgAlloc %v", err)
	}

	ist := assert(iCtx.GetStream(srcVideoStream.Index())).(*gmf.Stream)
	defer gmf.Release(ist)

	codecCtx := ist.CodecCtx()
	defer gmf.Release(codecCtx)

	//app.Log().Info("Entering get video packets loop...")

	for pkt := range iCtx.GetNewPackets() {

		if pkt.StreamIndex() != srcVideoStream.Index() {
			app.Log().Info("Skipping wrong stream packet")
			continue
		}

		frame, err := pkt.Frames(codecCtx)
		if err != nil {
			app.Log().Info("CodeCtx %v", err)
			continue
		}

		swsCtx.Scale(frame, dstFrame)

		p, err := dstFrame.Encode(cc)

		if err != nil {
			app.Log().Fatal("Encode %v", err)
		}
		rgba := new(image.RGBA)
		rgba.Stride = 4 * videoWidth
		rgba.Rect = image.Rect(0, 0, videoWidth, videoHeight)
		rgba.Pix = p.Data()

		// NO RACE, but slow...
		// select {
		// case app.picChan <- rgba:
		// default:
		// }

		// RACE...
		//app.Dispatch("feedUpdate", rgba)

		app.picMu.Lock()
		app.pic = rgba
		app.picMu.Unlock()

		// RAce...
		//app.Dispatch("feedUpdate", nil)

		gmf.Release(p)
		gmf.Release(frame)
		gmf.Release(pkt)

	}
}

// updateFeedTCB runs periodically to update the video feed image on the GUI.
func (app *tdApp) updateFeedTCB(cb interface{}) {
	// no race, but slower...
	// select {
	// case tmpPic := <-app.picChan:
	// 	app.texture.SetFromRGBA(tmpPic)
	// 	app.feed.SetChanged(true)
	// default:
	// }

	app.picMu.RLock()
	app.texture.SetFromRGBA(app.pic)
	app.picMu.RUnlock()
	app.feed.SetChanged(true)
}

// func (app *tdApp) feedUpdateCB(s string, ev interface{}) {
// 	app.texture.SetFromRGBA(ev.(*image.RGBA))
// }
// func (app *tdApp) feedUpdateCB(s string, ev interface{}) {
// 	app.picMu.RLock()
// 	app.texture.SetFromRGBA(app.pic)
// 	app.picMu.RUnlock()
// 	app.feed.SetChanged(true)
// }
