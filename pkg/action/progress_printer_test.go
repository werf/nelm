package action

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("progress printer", func() {
	Describe("unit testing", func() {
		Describe("Stop()", func() {
			It("should do nothing if not started", func(ctx SpecContext) {
				progressPrinter := newProgressTablePrinter(ctx, 0, 0, func() {
					// do nothing
				})
				Eventually(progressPrinter.Stop).WithTimeout(time.Second)
			})
		})
		Describe("Wait()", func() {
			It("should do nothing if not started", func(ctx SpecContext) {
				progressPrinter := newProgressTablePrinter(ctx, 0, 0, func() {
					// do nothing
				})
				Eventually(progressPrinter.Wait).WithTimeout(time.Second)
			})
		})
	})
	DescribeTable("functional testing",
		func(ctx SpecContext, cancelTimeout, stopTimeout, timeout, interval time.Duration, expectedTimes int) {
			ctxNew, cancel := context.WithTimeout(ctx, cancelTimeout)
			defer cancel()

			counter := 0
			progressPrinter := newProgressTablePrinter(ctxNew, interval, timeout, func() {
				counter++
			})

			progressPrinter.Start()

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
})
