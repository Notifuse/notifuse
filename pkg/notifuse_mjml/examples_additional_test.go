package notifuse_mjml

import (
	"os"
	"strings"
	"testing"
)

func TestNewBaseBlock(t *testing.T) {
	b := NewBaseBlock("x", MJMLComponentMjText)
	if b.ID != "x" || b.Type != MJMLComponentMjText {
		t.Fatalf("unexpected base block: %+v", b)
	}
	if b.Children == nil || b.Attributes == nil {
		t.Fatalf("children/attributes should be initialized")
	}
}

func TestCreateSimpleEmailAndConversion(t *testing.T) {
	email := CreateSimpleEmail()
	if err := ValidateEmailStructure(email); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	out := ConvertJSONToMJML(email)
	if !strings.Contains(out, "<mjml>") || !strings.Contains(out, "<mj-body") {
		t.Fatalf("unexpected MJML: %s", out)
	}
}

func TestCreateEmailWithImage(t *testing.T) {
	email := CreateEmailWithImage()
	out := ConvertJSONToMJML(email)
	if !strings.Contains(out, "<mj-image") {
		t.Fatalf("expected image block in MJML: %s", out)
	}
}

func TestCreateSocialEmail(t *testing.T) {
	email := CreateSocialEmail()
	out := ConvertJSONToMJML(email)
	if !strings.Contains(out, "<mj-social") || !strings.Contains(out, "<mj-social-element") {
		t.Fatalf("expected social blocks in MJML: %s", out)
	}
}

func TestConvertToEmailBuilderState_CreateSavedBlock(t *testing.T) {
	email := CreateSimpleEmail()
	st := ConvertToEmailBuilderState(email)
	if st == nil || len(st.History) != 1 || st.HistoryIndex != 0 {
		t.Fatalf("unexpected builder state: %+v", st)
	}

	sb := CreateSavedBlock("id1", "name1", email)
	if sb == nil || sb.Block == nil || sb.Created == nil || sb.Updated == nil {
		t.Fatalf("unexpected saved block: %+v", sb)
	}
}

func TestPrintAndValidateHelpers(t *testing.T) {
	email := CreateSimpleEmail()
	// Silence stdout to avoid noisy test output
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	ValidateAndPrintEmail(email)
	PrintEmailStructure(email, 0)
	_ = w.Close()
	os.Stdout = old
	_ = r.Close()
}

func TestPointerHelpers(t *testing.T) {
	if p := boolPtr(true); p == nil || *p != true {
		t.Fatalf("boolPtr failed")
	}
	if p := intPtr(3); p == nil || *p != 3 {
		t.Fatalf("intPtr failed")
	}
}

func TestDemoFunctions(t *testing.T) {
	// Just ensure they run without panic
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	DemoConverter()
	ConvertEmailToMJMLDemo()
	TestConverterFunctions()
	_ = w.Close()
	os.Stdout = old
	_ = r.Close()
}
