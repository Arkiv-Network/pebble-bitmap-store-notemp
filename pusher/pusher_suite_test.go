package pusher_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPusher(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pusher Suite")
}
