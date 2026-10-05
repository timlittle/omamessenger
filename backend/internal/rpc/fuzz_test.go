package rpc

import "testing"

func FuzzDecode(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(`{"id":1,"method":"hello","params":{}}`),
		[]byte(`{"id":0,"method":"hello"}`),
		[]byte(`not json`),
		[]byte(`[]`),
		[]byte(`{"id":-1,"method":"hello","params":{}}`),
		[]byte(`{"id":2,"method":"hello","params":[]}`),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = decodeRequest(data)
	})
}
