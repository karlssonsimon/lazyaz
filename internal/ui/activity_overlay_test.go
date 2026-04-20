package ui

import "testing"

func TestActivityOverlayClosedByDefault(t *testing.T) {
	var s ActivityOverlayState
	if s.Active {
		t.Fatalf("should start closed")
	}
}

func TestActivityOverlayOpenSetsListView(t *testing.T) {
	var s ActivityOverlayState
	s.Open()
	if !s.Active {
		t.Fatalf("Open should set Active=true")
	}
	if s.View != ActivityListPane {
		t.Fatalf("Open should default to list view")
	}
}

func TestActivityOverlayOpenDetailSetsDetailView(t *testing.T) {
	var s ActivityOverlayState
	s.OpenDetail("upload:abc")
	if !s.Active || s.View != ActivityDetailPane {
		t.Fatalf("OpenDetail should open in detail view")
	}
	if s.FocusedID != "upload:abc" {
		t.Fatalf("want FocusedID=upload:abc, got %q", s.FocusedID)
	}
}

func TestActivityOverlayCloseResets(t *testing.T) {
	var s ActivityOverlayState
	s.OpenDetail("upload:abc")
	s.Close()
	if s.Active {
		t.Fatalf("Close should clear Active")
	}
	if s.FocusedID != "" {
		t.Fatalf("Close should clear FocusedID")
	}
}

func TestActivityOverlayListKeyJMovesCursor(t *testing.T) {
	var s ActivityOverlayState
	s.Open()
	s.VisibleIDs = []string{"a", "b", "c"}
	s.HandleKey("j")
	if s.FocusedID != "b" {
		t.Fatalf("after j: want b, got %q", s.FocusedID)
	}
	s.HandleKey("j")
	s.HandleKey("j") // clamp
	if s.FocusedID != "c" {
		t.Fatalf("after 3j: want c (clamped), got %q", s.FocusedID)
	}
	s.HandleKey("k")
	if s.FocusedID != "b" {
		t.Fatalf("after k: want b, got %q", s.FocusedID)
	}
}

func TestActivityOverlayListEnterOpensDetail(t *testing.T) {
	var s ActivityOverlayState
	s.Open()
	s.VisibleIDs = []string{"a", "b"}
	s.FocusedID = "b"
	res := s.HandleKey("enter")
	if res.Action != ActivityActionDrill {
		t.Fatalf("enter in list: want Drill, got %v", res.Action)
	}
	if s.View != ActivityDetailPane {
		t.Fatalf("after enter: want DetailView")
	}
}

func TestActivityOverlayListEscCloses(t *testing.T) {
	var s ActivityOverlayState
	s.Open()
	res := s.HandleKey("esc")
	if res.Action != ActivityActionClose {
		t.Fatalf("esc in list: want Close, got %v", res.Action)
	}
	if s.Active {
		t.Fatalf("esc should close the overlay")
	}
}

func TestActivityOverlayDetailEscBacksToList(t *testing.T) {
	var s ActivityOverlayState
	s.OpenDetail("x")
	res := s.HandleKey("esc")
	if res.Action != ActivityActionBack {
		t.Fatalf("esc in detail: want Back, got %v", res.Action)
	}
	if s.View != ActivityListPane {
		t.Fatalf("esc in detail: want ListView")
	}
	if !s.Active {
		t.Fatalf("detail esc should NOT close the overlay")
	}
}

func TestActivityOverlayListXCancelsSelected(t *testing.T) {
	var s ActivityOverlayState
	s.Open()
	s.VisibleIDs = []string{"a"}
	s.FocusedID = "a"
	res := s.HandleKey("x")
	if res.Action != ActivityActionCancel {
		t.Fatalf("x: want Cancel action, got %v", res.Action)
	}
	if res.TargetID != "a" {
		t.Fatalf("x: want target a, got %q", res.TargetID)
	}
}
