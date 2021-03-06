/**
 *Copyright (c) 2018 Stephen Merrony
 *
 *This software is released under the MIT License.
 *https://opensource.org/licenses/MIT
 */

package main

import (
	"fmt"
	"image"

	"io"
	"log"
	"os"

	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/3d0c/gmf"
	"github.com/Anty0/tello"

	"github.com/mattn/go-gtk/gdk"
	"github.com/mattn/go-gtk/gdkpixbuf"
	"github.com/mattn/go-gtk/glib"
	"github.com/mattn/go-gtk/gtk"
)

const (
	videoScale                          = 1.45 //1.4125
	normalVideoWidth, normalVideoHeight = (int)(960 * videoScale), (int)(720 * videoScale)
	wideVideoWidth, wideVideoHeight     = (int)(1280 * videoScale), (int)(720 * videoScale)

	packetQueueLimit = 5000
)

type videoPacket struct {
	packet []byte
	next   *videoPacket
}

var (
	videoRecMu      sync.RWMutex
	videoWriteRecMu sync.RWMutex

	videoRecording bool

	videoConverter *exec.Cmd
	videoWriter    io.WriteCloser

	firstPacket *videoPacket
	lastPacket  *videoPacket
	packetLen   int
)

type videoWgtT struct {
	*gtk.Layout    // use a layout so we can overlay a message etc.
	image          *gtk.Image
	feedImage      *image.RGBA
	newFeedImageMu sync.Mutex
	newFeedImage   bool
	message        *gtk.Label
}

func buildVideodWgt() (wgt *videoWgtT) {
	wgt = new(videoWgtT)
	wgt.Layout = gtk.NewLayout(nil, nil)
	wgt.image = gtk.NewImageFromPixbuf(blueSkyPixbuf)
	//wgt.image.SetSizeRequest(videoWidth, videoHeight)
	wgt.Add(wgt.image)
	wgt.message = gtk.NewLabel("")
	wgt.message.ModifyFontEasy("Sans 20")
	wgt.message.ModifyFG(gtk.STATE_NORMAL, gdk.NewColor("red"))
	wgt.Put(wgt.message, 50, 50)
	return wgt
}

func (wgt *videoWgtT) setMessage(msg string) {
	wgt.message.SetText(msg)
}

func (wgt *videoWgtT) clearMessage() {
	wgt.message.SetText("")
}

func recordVideoCB() {
	videoRecMu.Lock()
	if !videoRecording {
		videoFilename := fmt.Sprintf("%s%ctello_vid_%s", settings.DataDir, filepath.Separator, time.Now().Format(time.RFC3339))
		videoConverter = exec.Command("ffmpeg", "-f", "pulse", "-i", "default", "-r", "30", "-i", "-", "-af", "aresample=async=1:first_pts=0", "-vcodec", "copy", videoFilename+".avi")

		var err error

		videoWriteRecMu.Lock()
		videoWriter, err = videoConverter.StdinPipe()
		videoWriteRecMu.Unlock()
		if err != nil {
			videoRecMu.Unlock()
			messageDialog(win, gtk.MESSAGE_INFO, "Could not prepare video converter.")
			return
		}

		err = videoConverter.Start()
		if err != nil {
			videoRecMu.Unlock()
			messageDialog(win, gtk.MESSAGE_INFO, "Could not start video converter.")
			return
		}

		firstPacket = nil
		lastPacket = nil
		packetLen = 0

		videoRecording = true
	}
	videoRecMu.Unlock()

	go videoWriterLoop()

	menuBar.recVidItem.SetSensitive(false)
	menuBar.stopRecVidItem.SetSensitive(true)
}

func stopRecordingVideoCB() {
	videoRecMu.Lock()
	videoRecording = false

	firstPacket = nil
	lastPacket = nil
	packetLen = 0

	videoWriteRecMu.Lock()
	videoWriter.Close()
	videoWriteRecMu.Unlock()

	videoConverter.Process.Signal(os.Interrupt)

	videoConverterDone := make(chan error)
	go func() { videoConverterDone <- videoConverter.Wait() }()

	videoConverterTimeout := time.After(15 * time.Second)

	select {
	case <-videoConverterTimeout:
		// Timeout happened first, kill the process and print a message.
		videoConverter.Process.Kill()
		log.Println("Failed to gracefully interrupt video converter")
	case <-videoConverterDone:
		// Convertor exited before timeout
	}
	videoRecMu.Unlock()

	menuBar.recVidItem.SetSensitive(true)
	menuBar.stopRecVidItem.SetSensitive(false)
}

func (wgt *videoWgtT) startVideo() {

	var err error

	videoChan, err = drone.VideoConnectDefault()
	if err != nil {
		log.Print(err.Error())
		messageDialog(win, gtk.MESSAGE_ERROR, err.Error())
	}

	drone.SetVideoBitrate(tello.VbrAuto)

	if settings.WideVideo {
		drone.SetVideoWide()
	}

	// start video SPS/PPS requestor when drone connects
	drone.GetVideoSpsPps()
	go func() { // no GTK stuff in here...
		for {
			drone.GetVideoSpsPps()
			select {
			case <-vrStopChan:
				return
			default:
			}
			time.Sleep(1000 * time.Millisecond)
		}
	}()

	stopFeedImageChan = make(chan bool)

	go wgt.videoListener()

	glib.TimeoutAdd(30, wgt.updateFeed)
}

func customReader() ([]byte, int) {
	pkt, more := <-videoChan
	if !more {
		stopFeedImageChan <- true
	}
	videoRecMu.Lock()
	if videoRecording {
		if packetLen < packetQueueLimit {
			if lastPacket == nil {
				lastPacket = new(videoPacket)
				firstPacket = lastPacket
			} else {
				lastPacket.next = new(videoPacket)
				lastPacket = lastPacket.next
			}

			lastPacket.next = nil
			lastPacket.packet = pkt

			packetLen++
		} else {
			log.Println("WARNING: Recording packet queue reached it's limit.")
		}
	}
	videoRecMu.Unlock()
	return pkt, len(pkt)
}

func assert(i interface{}, err error) interface{} {
	if err != nil {
		log.Fatalf("Assert %v", err)
	}

	return i
}

func videoWriterLoop() {
	for {
		videoRecMu.Lock()

		if !videoRecording {
			videoRecMu.Unlock()
			break
		}

		if firstPacket == nil {
			videoRecMu.Unlock()
			time.Sleep(5 * time.Millisecond)
			continue
		}

		pkt := firstPacket.packet

		firstPacket = firstPacket.next
		if firstPacket == nil {
			lastPacket = nil
		}
		packetLen--

		videoRecMu.Unlock()

		videoWriteRecMu.Lock()
		videoWriter.Write(pkt)
		videoWriteRecMu.Unlock()
	}
}

//func (app *tdApp) videoListener() {
func (wgt *videoWgtT) videoListener() {
	iCtx := gmf.NewCtx()
	defer iCtx.CloseInputAndRelease()

	if err := iCtx.SetInputFormat("h264"); err != nil {
		log.Fatalf("iCtx SetInputFormat %v", err)
	}

	avioCtx, err := gmf.NewAVIOContext(iCtx, &gmf.AVIOHandlers{ReadPacket: customReader})
	defer gmf.Release(avioCtx)
	if err != nil {
		log.Fatalf("NewAVIOContext %v", err)
	}

	iCtx.SetPb(avioCtx)

	err = iCtx.OpenInput("")
	if err != nil {
		log.Fatalf("iCtx OpenInput %v", err)
	}

	srcVideoStream, err := iCtx.GetBestStream(gmf.AVMEDIA_TYPE_VIDEO)
	if err != nil {
		log.Fatalf("GetBestStream %v", err)
	}

	codec, err := gmf.FindEncoder(gmf.AV_CODEC_ID_RAWVIDEO)
	if err != nil {
		log.Fatalf("FindDecoder %v", err)
	}
	cc := gmf.NewCodecCtx(codec)
	defer gmf.Release(cc)

	if codec.IsExperimental() {
		cc.SetStrictCompliance(gmf.FF_COMPLIANCE_EXPERIMENTAL)
	}

	cc.SetPixFmt(gmf.AV_PIX_FMT_BGR32).
		SetWidth(videoWidth).
		SetHeight(videoHeight).
		SetTimeBase(gmf.AVR{Num: 1, Den: 1})

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

	for pkt := range iCtx.GetNewPackets() {

		if pkt.StreamIndex() != srcVideoStream.Index() {
			log.Println("Skipping wrong stream packet")
			continue
		}

		frame, err := pkt.Frames(codecCtx)
		if err != nil {
			log.Printf("CodeCtx %v", err)
			continue
		}

		swsCtx.Scale(frame, dstFrame)

		p, err := dstFrame.Encode(cc)

		if err != nil {
			log.Fatalf("Encode %v", err)
		}
		rgba := new(image.RGBA)
		rgba.Stride = 4 * videoWidth
		rgba.Rect = image.Rect(0, 0, videoWidth, videoHeight)
		rgba.Pix = p.Data()

		wgt.newFeedImageMu.Lock()
		wgt.feedImage = rgba
		wgt.newFeedImage = true
		wgt.newFeedImageMu.Unlock()

		gmf.Release(p)
		gmf.Release(frame)
		gmf.Release(pkt)

	}
}

// updateFeed actually updates the video image in the feed tab.
// It must be run on the main thread, so there is a little mutex dance to
// check if a new image is ready for display.
func (wgt *videoWgtT) updateFeed() bool {
	wgt.newFeedImageMu.Lock()
	if wgt.newFeedImage {
		var pbd gdkpixbuf.PixbufData
		pbd.Colorspace = gdkpixbuf.GDK_COLORSPACE_RGB
		pbd.HasAlpha = true
		pbd.BitsPerSample = 8
		pbd.Width = videoWidth
		pbd.Height = videoHeight
		pbd.RowStride = videoWidth * 4 // RGBA

		pbd.Data = wgt.feedImage.Pix

		pb := gdkpixbuf.NewPixbufFromData(pbd)
		//pb = pb.ScaleSimple(videoWidth, videoHeight, gdkpixbuf.INTERP_BILINEAR)
		videoWgt.image.SetFromPixbuf(pb)

		wgt.newFeedImage = false
	}
	wgt.newFeedImageMu.Unlock()

	// check if feed should be shutdown
	select {
	case <-stopFeedImageChan:
		log.Println("Debug: updateFeed stopping")
		wgt.image.SetFromPixbuf(blueSkyPixbuf)
		return false // stops the timer
	default:
	}
	return true // continues the timer
}
