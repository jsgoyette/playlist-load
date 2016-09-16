package main

import (
	"testing"
)

func TestNewId(t *testing.T) {
	id := NewId(20)
	if len(id) != 20 {
		t.Error("expected 20, got ", id)
	}
}
