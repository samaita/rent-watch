package extractor

import (
	"context"
	"time"

	"github.com/chromedp/chromedp"
)

type BrowserOptions struct {
	Headless bool
	ExecPath string
	Timeout  time.Duration
}

func newBrowserContext(parent context.Context, opts BrowserOptions) (context.Context, context.CancelFunc) {
	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", opts.Headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)
	if opts.ExecPath != "" {
		allocOpts = append(allocOpts, chromedp.ExecPath(opts.ExecPath))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(parent, allocOpts...)
	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	ctx, timeoutCancel := context.WithTimeout(taskCtx, timeout)

	return ctx, func() {
		timeoutCancel()
		taskCancel()
		allocCancel()
	}
}
