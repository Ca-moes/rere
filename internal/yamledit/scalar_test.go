package yamledit

import (
	"bytes"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func qEq(t *testing.T, got *resource.Quantity, want string) {
	t.Helper()
	if got == nil {
		t.Errorf("got nil, want %q", want)
		return
	}
	w := resource.MustParse(want)
	if got.Cmp(w) != 0 {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestReadCurrent(t *testing.T) {
	in := readFixture(t, "removelimit.in.yaml")
	rec, err := ReadCurrent(bytes.NewReader(in), "Deployment", "web", "web")
	if err != nil {
		t.Fatalf("ReadCurrent: %v", err)
	}
	qEq(t, rec.Requests.CPU, "100m")
	qEq(t, rec.Requests.Mem, "64Mi")
	qEq(t, rec.Limits.CPU, "200m")
	qEq(t, rec.Limits.Mem, "128Mi")
}

func TestReadCurrent_ContainerNotFound(t *testing.T) {
	in := readFixture(t, "removelimit.in.yaml")
	rec, err := ReadCurrent(bytes.NewReader(in), "Deployment", "web", "ghost")
	if err != nil {
		t.Fatalf("ReadCurrent: %v", err)
	}
	if rec.Requests.CPU != nil || rec.Requests.Mem != nil || rec.Limits.CPU != nil || rec.Limits.Mem != nil {
		t.Errorf("expected empty Recommended for missing container, got %+v", rec)
	}
}

func TestReadCurrentAt(t *testing.T) {
	in := readFixture(t, "removelimit.in.yaml")
	root, err := SelectDoc(in, "Deployment", "web")
	if err != nil || root == nil {
		t.Fatalf("SelectDoc: err=%v root=%v", err, root)
	}
	rec, err := ReadCurrentAt(root, func(section, res string) ([]string, error) {
		return []string{"spec", "template", "spec", "containers", "[name=web]", "resources", section, res}, nil
	})
	if err != nil {
		t.Fatalf("ReadCurrentAt: %v", err)
	}
	qEq(t, rec.Requests.CPU, "100m")
	qEq(t, rec.Requests.Mem, "64Mi")
	qEq(t, rec.Limits.CPU, "200m")
	qEq(t, rec.Limits.Mem, "128Mi")
}

func TestReadCurrent_PartialResources(t *testing.T) {
	in := readFixture(t, "notfound.in.yaml") // web: requests.cpu only
	rec, err := ReadCurrent(bytes.NewReader(in), "Deployment", "web", "web")
	if err != nil {
		t.Fatalf("ReadCurrent: %v", err)
	}
	qEq(t, rec.Requests.CPU, "100m")
	if rec.Requests.Mem != nil || rec.Limits.CPU != nil || rec.Limits.Mem != nil {
		t.Errorf("expected only requests.cpu, got %+v", rec)
	}
}
