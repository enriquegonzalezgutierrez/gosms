package mobile

import (
	"bytes"
	"testing"
)

func TestAudioBufferWriteRead(t *testing.T) {
	buf := newAudioBuffer()
	input := []byte{1, 2, 3, 4}
	buf.Write(input)

	output := make([]byte, len(input))
	n, err := buf.Read(output)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if n != len(input) {
		t.Fatalf("Read() n = %d, want %d", n, len(input))
	}
	if !bytes.Equal(output, input) {
		t.Fatalf("Read() = %v, want %v", output, input)
	}
}

func TestAudioBufferReadConsumesOnlyRequestedBytes(t *testing.T) {
	buf := newAudioBuffer()
	buf.Write([]byte{1, 2, 3, 4})

	first := make([]byte, 2)
	if n, err := buf.Read(first); err != nil || n != 2 {
		t.Fatalf("first Read() = (%d, %v), want (2, nil)", n, err)
	}
	if !bytes.Equal(first, []byte{1, 2}) {
		t.Fatalf("first Read() = %v, want [1 2]", first)
	}
	second := make([]byte, 2)
	if n, err := buf.Read(second); err != nil || n != 2 {
		t.Fatalf("second Read() = (%d, %v), want (2, nil)", n, err)
	}
	if !bytes.Equal(second, []byte{3, 4}) {
		t.Fatalf("second Read() = %v, want [3 4]", second)
	}
}
