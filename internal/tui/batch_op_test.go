package tui

import (
	"errors"
	"testing"
)

func TestBatchOp_Complete(t *testing.T) {
	items := []batchItem{
		{name: "a", status: batchPending},
		{name: "b", status: batchPending},
	}
	b := newBatchOp("Testing", items)

	b.items[0].status = batchRunning
	b.complete(0, nil)
	if b.items[0].status != batchDone {
		t.Errorf("expected batchDone, got %d", b.items[0].status)
	}
	if b.done != 1 {
		t.Errorf("expected done=1, got %d", b.done)
	}

	b.items[1].status = batchRunning
	b.complete(1, errors.New("fail"))
	if b.items[1].status != batchFailed {
		t.Errorf("expected batchFailed, got %d", b.items[1].status)
	}
	if b.done != 2 {
		t.Errorf("expected done=2, got %d", b.done)
	}
}

func TestBatchOp_SkipNextPending(t *testing.T) {
	items := []batchItem{
		{name: "a", status: batchRunning},
		{name: "b", status: batchPending},
		{name: "c", status: batchPending},
	}
	b := newBatchOp("Testing", items)

	// Skip targets the next pending item (not the running one).
	if !b.skip() {
		t.Error("skip should return true")
	}
	if b.items[0].status != batchRunning {
		t.Errorf("running item should stay running, got %d", b.items[0].status)
	}
	if b.items[1].status != batchSkipped {
		t.Errorf("first pending item should be skipped, got %d", b.items[1].status)
	}
	if b.done != 1 {
		t.Errorf("expected done=1, got %d", b.done)
	}

	// Skip again — targets item c.
	if !b.skip() {
		t.Error("second skip should return true")
	}
	if b.items[2].status != batchSkipped {
		t.Errorf("second pending item should be skipped, got %d", b.items[2].status)
	}

	// No more pending — skip returns false.
	if b.skip() {
		t.Error("no pending items, skip should return false")
	}
}

func TestBatchOp_Cancel(t *testing.T) {
	items := []batchItem{
		{name: "a", status: batchRunning},
		{name: "b", status: batchPending},
		{name: "c", status: batchPending},
	}
	b := newBatchOp("Testing", items)

	b.cancel()
	if !b.cancelled {
		t.Error("expected cancelled=true")
	}
	// Running item stays running (process still executing).
	if b.items[0].status != batchRunning {
		t.Errorf("running item should stay running, got %d", b.items[0].status)
	}
	// Pending items should be skipped.
	if b.items[1].status != batchSkipped || b.items[2].status != batchSkipped {
		t.Error("pending items should be skipped")
	}
	if b.done != 2 {
		t.Errorf("expected done=2 (2 cancelled), got %d", b.done)
	}
}

func TestBatchOp_Summary(t *testing.T) {
	items := []batchItem{
		{name: "a", status: batchDone},
		{name: "b", status: batchFailed, errMsg: "err"},
		{name: "c", status: batchSkipped, errMsg: "skipped"},
	}
	b := newBatchOp("Testing", items)
	s := b.summary()
	if s != "⚠ 1 succeeded, 1 failed, 1 skipped" {
		t.Errorf("unexpected summary: %q", s)
	}
}

func TestBatchOp_SummaryCancelled(t *testing.T) {
	items := []batchItem{
		{name: "a", status: batchDone},
		{name: "b", status: batchSkipped, errMsg: "cancelled"},
	}
	b := newBatchOp("Testing", items)
	b.cancelled = true
	s := b.summary()
	if s != "⚠ Cancelled — 1 succeeded, 1 skipped" {
		t.Errorf("unexpected summary: %q", s)
	}
}

func TestBatchOp_Progress(t *testing.T) {
	items := []batchItem{
		{name: "a", status: batchDone},
		{name: "b", status: batchRunning},
		{name: "c", status: batchPending},
	}
	b := newBatchOp("Testing", items)
	b.done = 1
	if b.progress() != "1/3" {
		t.Errorf("expected 1/3, got %s", b.progress())
	}
}
