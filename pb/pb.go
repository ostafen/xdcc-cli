package pb

import (
	"time"
	"xdcc-cli/util"

	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
)

type ProgressState string

const (
	ProgressStateConnecting  ProgressState = "connecting"
	ProgressStateDownloading ProgressState = "downloading"
	ProgressStateCompleted   ProgressState = "done"
	ProgressStateAborted     ProgressState = "aborted"
)

type ProgressBar interface {
	Increment(n int)
	SetTotal(n int)
	SetFileName(fileName string)
	SetState(state ProgressState)
}

type progressBarImpl struct {
	progress *mpb.Progress
	*mpb.Bar
	state    ProgressState
	total    int
	fileName string
}

const (
	barRefreshRateDefault = 180 * time.Millisecond
	barWidthDefault       = 40
	barMaxFileNameWidth   = 35
)

func createMpbBar(p *mpb.Progress, total int, taskName string, state ProgressState, queueBar *mpb.Bar) *mpb.Bar {
	displayName := util.CutStr(taskName, barMaxFileNameWidth)

	len := len(displayName)
	if len != 0 {
		displayName += ":"
		len += 2
	}

	options := []mpb.BarOption{
		mpb.PrependDecorators(
			decor.Name(displayName, decor.WC{W: len, C: decor.DidentRight}),
			decor.Name(string(state), decor.WCSyncSpaceR),
			decor.CountersKibiByte("% .2f / % .2f"),
		),
		mpb.AppendDecorators(
			decor.EwmaETA(decor.ET_STYLE_GO, 90),
			decor.Name(" ] "),
			decor.EwmaSpeed(decor.UnitKiB, "% .2f", 60),
		),
	}

	if queueBar != nil {
		options = append([]mpb.BarOption{
			mpb.BarQueueAfter(queueBar),
			mpb.BarFillerClearOnComplete(),
		}, options...)
	}

	return p.Add(int64(total),
		mpb.NewBarFiller(mpb.BarStyle().Rbound("|")),
		options...,
	)
}

var progress *mpb.Progress = nil

func init() {
	progress = mpb.New(
		mpb.WithWidth(barWidthDefault),
		mpb.WithRefreshRate(barRefreshRateDefault),
	)
}

func newProgressBarImpl() *progressBarImpl {
	bar := createMpbBar(progress, 0, "", ProgressStateConnecting, nil)
	return &progressBarImpl{
		progress: progress,
		total:    0,
		state:    ProgressStateConnecting,
		Bar:      bar,
	}
}

func (bar *progressBarImpl) SetTotal(n int) {
	bar.total = n
	bar.Bar.SetTotal(int64(n), false)
}

func (bar *progressBarImpl) SetFileName(fileName string) {
	bar.fileName = fileName
}

func (bar *progressBarImpl) Increment(n int) {
	bar.IncrBy(n)
	bar.DecoratorEwmaUpdate(time.Second)
}

func (bar *progressBarImpl) SetState(state ProgressState) {
	if state != bar.state {
		oldBar := bar.Bar
		bar.Bar = createMpbBar(bar.progress, bar.total, bar.fileName, state, bar.Bar)
		oldBar.SetTotal(0, true)
	}
}

func NewProgressBar() ProgressBar {
	return newProgressBarImpl()
}
