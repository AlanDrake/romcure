package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSerialListsAreStoredAndAFailureOnlyNoted(t *testing.T) {
	a := newApp(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "PlayStation.dat") {
			w.Write([]byte("clrmamepro (\n\tversion \"1\"\n)\ngame (\n\tname \"Crash Bandicoot (USA)\"\n\tserial \"SCUS-94900\"\n)\n"))
			return
		}
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer srv.Close()
	ep := Endpoints{Client: srv.Client(), SerialsURL: srv.URL + "/"}

	if msg := a.updateSerials(context.Background(), ep, "psx"); msg != " (+1 serials)" {
		t.Errorf("message = %q", msg)
	}
	if name, ok, _ := a.DB.SerialName("psx", "SCUS-94900"); !ok || name != "Crash Bandicoot (USA)" {
		t.Errorf("stored name = %q %v", name, ok)
	}
	if msg := a.updateSerials(context.Background(), ep, "ps2"); !strings.Contains(msg, "not updated") {
		t.Errorf("a failed download must be noted: %q", msg)
	}
}
