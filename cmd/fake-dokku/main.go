// Command fake-dokku stands in for the dokku binary on a machine without Dokku. It answers
// each call from the same seeded state as GOKKU_RUNNER=fake, in Dokku's output formats,
// so Gokku's exec runner can be tried end to end:
//
//	go tool task fake-dokku
//	GOKKU_DOKKU_BIN=$PWD/bin/fake-dokku GOKKU_DOKKU_SUDO=false ./bin/gokku serve --data-dir .dev-data
//
// While a file named like the binary plus ".fail" exists (bin/fake-dokku.fail), every call
// fails the way an unreachable Dokku does, with the file's content as the error. State is
// not kept between calls. It is a development tool and is not part of a release.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"gokku/internal/dokku"
)

func main() {
	fake := dokku.NewFakeRunner(time.Now())
	if exe, err := os.Executable(); err == nil {
		fake.FailFile = exe + ".fail"
	}
	res, err := fake.Run(context.Background(), os.Args[1:]...)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_, _ = os.Stdout.WriteString(res.Stdout)
	_, _ = os.Stderr.WriteString(res.Stderr)
	os.Exit(res.ExitCode)
}
