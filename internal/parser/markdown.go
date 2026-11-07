package parser

import (
	"bytes"
	"sync"
)

// sync.Pool untuk bytes.Buffer guna mengeliminasi alokasi berulang pada throughput tinggi.
var bufferPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 4096))
	},
}

func getBuffer() *bytes.Buffer {
	b := bufferPool.Get().(*bytes.Buffer)
	b.Reset()
	return b
}

func putBuffer(b *bytes.Buffer) {
	if b == nil {
		return
	}
	if b.Cap() > 65536 {
		// Hindari menyimpan buffer raksasa di pool agar memori tidak leak.
		return
	}
	b.Reset()
	bufferPool.Put(b)
}
