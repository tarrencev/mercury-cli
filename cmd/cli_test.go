package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestRoot(t *testing.T) (*bytes.Buffer, *bytes.Buffer, func(args ...string) error) {
	t.Helper()
	t.Setenv("MERCURY_TOKEN", "")
	t.Setenv("MERCURY_ENV", "")

	root, err := NewRootCmd()
	if err != nil {
		t.Fatalf("NewRootCmd: %v", err)
	}
	var out bytes.Buffer
	var errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)

	run := func(args ...string) error {
		root.SetArgs(args)
		return root.Execute()
	}
	return &out, &errBuf, run
}

func TestQueryFlagAliases(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"accounts":[],"page":{"nextPage":null,"previousPage":null}}`)
	}))
	t.Cleanup(srv.Close)

	{
		out, errBuf, run := newTestRoot(t)
		err := run("--token", "t", "--base-url", srv.URL+"/api/v1", "accounts", "get-accounts", "--start-after", "abc")
		if err != nil {
			t.Fatalf("execute: %v (stderr=%s)", err, errBuf.String())
		}
		if got.Get("start_after") != "abc" {
			t.Fatalf("expected start_after=abc, got %q (query=%v)", got.Get("start_after"), got)
		}
		if out.Len() == 0 {
			t.Fatalf("expected stdout body")
		}
	}

	{
		out, errBuf, run := newTestRoot(t)
		err := run("--token", "t", "--base-url", srv.URL+"/api/v1", "accounts", "get-accounts", "--start_after", "def")
		if err != nil {
			t.Fatalf("execute: %v (stderr=%s)", err, errBuf.String())
		}
		if got.Get("start_after") != "def" {
			t.Fatalf("expected start_after=def, got %q (query=%v)", got.Get("start_after"), got)
		}
		if out.Len() == 0 {
			t.Fatalf("expected stdout body")
		}
	}
}

func TestJSONBody(t *testing.T) {
	var gotCT string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)

	_, errBuf, run := newTestRoot(t)
	err := run("--token", "t", "--base-url", srv.URL+"/api/v1", "recipients", "create-recipient", "--data", `{"name":"x"}`)
	if err != nil {
		t.Fatalf("execute: %v (stderr=%s)", err, errBuf.String())
	}
	if !strings.HasPrefix(gotCT, "application/json") {
		t.Fatalf("expected JSON Content-Type, got %q", gotCT)
	}
	if string(gotBody) != `{"name":"x"}` {
		t.Fatalf("unexpected body: %q", string(gotBody))
	}
}

func TestFormURLEncodedBodyAndBasicAuth(t *testing.T) {
	var gotCT string
	var gotAuth string
	var gotForm url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotForm, _ = url.ParseQuery(string(b))
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"access_token":"x"}`)
	}))
	t.Cleanup(srv.Close)

	_, errBuf, run := newTestRoot(t)
	err := run("--token", "clientsecret", "--auth", "basic", "--base-url", srv.URL, "oauth2", "obtain-access-token",
		"--form", "grant_type=client_credentials",
		"--form", "scope=read",
	)
	if err != nil {
		t.Fatalf("execute: %v (stderr=%s)", err, errBuf.String())
	}
	if gotCT != "application/x-www-form-urlencoded" {
		t.Fatalf("expected form Content-Type, got %q", gotCT)
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Fatalf("expected Basic auth, got %q", gotAuth)
	}
	wantBasic := "Basic " + base64.StdEncoding.EncodeToString([]byte("clientsecret:"))
	if gotAuth != wantBasic {
		t.Fatalf("unexpected Authorization: got %q want %q", gotAuth, wantBasic)
	}
	if gotForm.Get("grant_type") != "client_credentials" || gotForm.Get("scope") != "read" {
		t.Fatalf("unexpected form: %v", gotForm)
	}
}

func TestMultipartUpload(t *testing.T) {
	tmpDir := t.TempDir()
	fpath := filepath.Join(tmpDir, "hello.txt")
	if err := os.WriteFile(fpath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	var gotFile string
	var gotNote string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		gotNote = r.FormValue("note")
		f, fh, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile: %v", err)
		}
		defer f.Close()
		b, _ := io.ReadAll(f)
		gotFile = fh.Filename + ":" + string(b)

		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)

	_, errBuf, run := newTestRoot(t)
	err := run("--token", "t", "--base-url", srv.URL+"/api/v1", "recipients", "upload-recipient-attachment", "r_123",
		"--form", "note=hi",
		"--form", "file=@"+fpath,
	)
	if err != nil {
		t.Fatalf("execute: %v (stderr=%s)", err, errBuf.String())
	}
	if gotNote != "hi" {
		t.Fatalf("expected note=hi, got %q", gotNote)
	}
	if gotFile != "hello.txt:hello" {
		t.Fatalf("unexpected uploaded file: %q", gotFile)
	}
}

func TestPaginationCursorAll(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.RawQuery)

		w.Header().Set("Content-Type", "application/json")
		startAfter := r.URL.Query().Get("start_after")
		if startAfter == "" {
			io.WriteString(w, `{"accounts":[{"id":"a1"},{"id":"a2"}],"page":{"nextPage":"t1","previousPage":null}}`)
			return
		}
		if startAfter == "t1" {
			io.WriteString(w, `{"accounts":[{"id":"a3"}],"page":{"nextPage":null,"previousPage":"t0"}}`)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"unexpected token"}`)
	}))
	t.Cleanup(srv.Close)

	out, errBuf, run := newTestRoot(t)
	err := run("--token", "t", "--base-url", srv.URL+"/api/v1", "accounts", "get-accounts", "--all")
	if err != nil {
		t.Fatalf("execute: %v (stderr=%s)", err, errBuf.String())
	}
	var obj map[string]any
	if err := json.Unmarshal(out.Bytes(), &obj); err != nil {
		t.Fatalf("parse output: %v (out=%s)", err, out.String())
	}
	accts, _ := obj["accounts"].([]any)
	if len(accts) != 3 {
		t.Fatalf("expected 3 accounts, got %d (out=%s)", len(accts), out.String())
	}
	if len(calls) < 2 {
		t.Fatalf("expected multiple calls, got %v", calls)
	}
}

func TestPaginationOffsetAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		offset := r.URL.Query().Get("offset")
		switch offset {
		case "", "0":
			io.WriteString(w, `{"total":3,"transactions":[{"id":"t1"},{"id":"t2"}]}`)
		case "2":
			io.WriteString(w, `{"total":3,"transactions":[{"id":"t3"}]}`)
		default:
			io.WriteString(w, `{"total":3,"transactions":[]}`)
		}
	}))
	t.Cleanup(srv.Close)

	out, errBuf, run := newTestRoot(t)
	err := run("--token", "t", "--base-url", srv.URL+"/api/v1", "accounts", "list-account-transactions", "acc_1", "--all")
	if err != nil {
		t.Fatalf("execute: %v (stderr=%s)", err, errBuf.String())
	}
	var obj map[string]any
	if err := json.Unmarshal(out.Bytes(), &obj); err != nil {
		t.Fatalf("parse output: %v (out=%s)", err, out.String())
	}
	tx, _ := obj["transactions"].([]any)
	if len(tx) != 3 {
		t.Fatalf("expected 3 transactions, got %d (out=%s)", len(tx), out.String())
	}
}

func TestPaginationPageTokenAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		token := r.URL.Query().Get("page_token")
		if token == "" {
			io.WriteString(w, `{"records":[{"id":"j1"}],"next_page_token":"n1"}`)
			return
		}
		if token == "n1" {
			io.WriteString(w, `{"records":[{"id":"j2"}],"next_page_token":null}`)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"bad token"}`)
	}))
	t.Cleanup(srv.Close)

	out, errBuf, run := newTestRoot(t)
	err := run("--token", "t", "--base-url", srv.URL+"/api/v1", "books", "get-books-journal-entries", "b_1", "--all")
	if err != nil {
		t.Fatalf("execute: %v (stderr=%s)", err, errBuf.String())
	}
	var obj map[string]any
	if err := json.Unmarshal(out.Bytes(), &obj); err != nil {
		t.Fatalf("parse output: %v (out=%s)", err, out.String())
	}
	recs, _ := obj["records"].([]any)
	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d (out=%s)", len(recs), out.String())
	}
}

func TestNDJSONAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		startAfter := r.URL.Query().Get("start_after")
		if startAfter == "" {
			io.WriteString(w, `{"accounts":[{"id":"a1"},{"id":"a2"}],"page":{"nextPage":"t1","previousPage":null}}`)
			return
		}
		io.WriteString(w, `{"accounts":[{"id":"a3"}],"page":{"nextPage":null,"previousPage":"t0"}}`)
	}))
	t.Cleanup(srv.Close)

	out, errBuf, run := newTestRoot(t)
	err := run("--token", "t", "--ndjson", "--base-url", srv.URL+"/api/v1", "accounts", "get-accounts", "--all")
	if err != nil {
		t.Fatalf("execute: %v (stderr=%s)", err, errBuf.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d (out=%q)", len(lines), out.String())
	}
	for _, ln := range lines {
		if !json.Valid([]byte(ln)) {
			t.Fatalf("invalid json line: %q", ln)
		}
	}
}

// Quick sanity: ensure our multipart parsing helper in server is not silently broken.
func TestMultipartServerParseSanity(t *testing.T) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("note", "hi")
	part, _ := w.CreateFormFile("file", "a.txt")
	part.Write([]byte("hello"))
	w.Close()
	ct := w.FormDataContentType()

	r := httptest.NewRequest(http.MethodPost, "http://example.com", &buf)
	r.Header.Set("Content-Type", ct)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		t.Fatal(err)
	}
	if r.FormValue("note") != "hi" {
		t.Fatal("bad note")
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(f)
	f.Close()
	if string(b) != "hello" {
		t.Fatal("bad file")
	}
}
