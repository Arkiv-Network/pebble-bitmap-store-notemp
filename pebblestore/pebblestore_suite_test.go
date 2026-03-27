package pebblestore_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPebbleStore(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PebbleStore Suite")
}
