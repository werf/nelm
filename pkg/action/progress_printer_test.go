package action

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = DescribeTable("progress printer",
	func(ctx SpecContext, cancelTimeout, stopTimeout, timeout, interval time.Duration, expectedTimes int) {
		ctxNew, cancel := context.WithTimeout(ctx, cancelTimeout)
		defer cancel()

		progressPrinter := newProgressTablePrinter(ctxNew, interval, timeout)

		counter := 0
		progressPrinter.Start(func() {
			counter++
		})

		if stopTimeout > 0 {
			time.Sleep(stopTimeout)
			progressPrinter.Stop()
		}

		progressPrinter.Wait()
		Expect(counter).To(Equal(expectedTimes))
	},
	Entry(
		"should stop using timeout and print 5 times with cancelTimeout=1min, stopTimeout=0, timeout=100ms, interval=25ms",
		time.Minute,
		time.Duration(0),
		time.Millisecond*100,
		time.Millisecond*25,
		5,
	),
	Entry(
		"should stop using ctx and print 3 times with cancelTimeout=50ms, stopTimeout=0, timeout=100ms, interval=25ms",
		time.Millisecond*50,
		time.Duration(0),
		time.Millisecond*100,
		time.Millisecond*20,
		3,
	),
	Entry(
		"should stop using Stop() and print 3 times with cancelTimeout=1min, stopTimeout=100ms, timeout=100ms, interval=25ms",
		time.Minute,
		time.Millisecond*50,
		time.Millisecond*100,
		time.Millisecond*25,
		3,
	),
	Entry(
		"should consider timeout=0 as 24 hours and print 3 times with cancelTimeout=1min, stopTimeout=50ms, timeout=0, interval=25ms",
		time.Minute,
		time.Millisecond*50,
		time.Duration(0),
		time.Millisecond*25,
		3,
	),
)
