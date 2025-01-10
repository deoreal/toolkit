package toolkit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
)

type RoundTripFunc func(req *http.Request) *http.Response

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func NewTestClient(fn RoundTripFunc) *http.Client {
	return &http.Client{
		Transport: fn,
	}
}
func TestTools_PushJSONToRemote(t *testing.T) {
	client := NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString("ok")),
			Header:     make(http.Header),
		}
	})

	var testTools Tools
	var foo struct {
		Bar string `json:"bar"`
	}
	foo.Bar = "bar"
	_, _, err := testTools.PushJSONToRemote("http://example.com/some/path", foo, client)
	if err != nil {
		t.Error("failed to call remote url", err)
	}

}

func TestTools_RandomString(t *testing.T) {
	var testTools Tools

	s := testTools.RandomString(10)
	if len(s) != 10 {
		t.Error("wrong length random string returned")
	}
}

var uploadTests = []struct {
	name          string
	allowedTypes  []string
	renameFile    bool
	errorExpected bool
}{
	{name: "allowed no rename", allowedTypes: []string{"image/jpeg", "image/png"}, renameFile: false, errorExpected: false},
	{name: "allowed rename", allowedTypes: []string{"image/jpeg", "image/png"}, renameFile: true, errorExpected: false},
	{name: "not allowed", allowedTypes: []string{"image/jpeg"}, renameFile: false, errorExpected: true},
}

func TestTools_UploadFiles(t *testing.T) {
	for _, e := range uploadTests {
		// set up a pipe to avoid buffering
		pr, pw := io.Pipe()
		writer := multipart.NewWriter(pw)
		wg := sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer writer.Close()
			defer wg.Done()

			/// create the form data field 'file'
			part, err := writer.CreateFormFile("file", "./testdata/img.png")
			if err != nil {
				t.Error(err)
			}

			f, err := os.Open("./testdata/img.png")
			if err != nil {
				t.Error(err)
			}
			defer f.Close()

			img, _, err := image.Decode(f)
			if err != nil {
				t.Error("error decoding image", err)
			}

			err = png.Encode(part, img)
			if err != nil {
				t.Error(err)
			}

		}()

		// read from the pipe which receives data
		request := httptest.NewRequest("POST", "/", pr)
		request.Header.Add("Content-Type", writer.FormDataContentType())

		var testTools Tools
		testTools.AllowedFileTypes = e.allowedTypes

		uploadedFiles, err := testTools.UploadFiles(request, "./testdata/uploads/", e.renameFile)
		if err != nil && !e.errorExpected {
			t.Error(err)
		}

		if !e.errorExpected {
			if _, err := os.Stat(fmt.Sprintf("./testdata/uploads/%s", uploadedFiles[0].NewFileName)); os.IsNotExist(err) {
				t.Errorf("%s: expected file to exist: %s", e.name, err.Error())
			}

			// clean up
			_ = os.Remove(fmt.Sprintf("./testdata/uploads/%s", uploadedFiles[0].NewFileName))
		}

		if !e.errorExpected && err != nil {
			t.Errorf("%s: error expected but none received", e.name)
		}

		wg.Wait()
	}
}

func TestTools_UploadOneFile(t *testing.T) {
	// set up a pipe to avoid buffering
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		defer writer.Close()

		/// create the form data field 'file'
		part, err := writer.CreateFormFile("file", "./testdata/img.png")
		if err != nil {
			t.Error(err)
		}

		f, err := os.Open("./testdata/img.png")
		if err != nil {
			t.Error(err)
		}
		defer f.Close()

		img, _, err := image.Decode(f)
		if err != nil {
			t.Error("error decoding image", err)
		}

		err = png.Encode(part, img)
		if err != nil {
			t.Error(err)
		}

	}()

	// read from the pipe which receives data
	request := httptest.NewRequest("POST", "/", pr)
	request.Header.Add("Content-Type", writer.FormDataContentType())

	var testTools Tools

	uploadedFiles, err := testTools.UploadOneFile(request, "./testdata/uploads/", true)
	if err != nil {
		t.Error(err)
	}

	if _, err := os.Stat(fmt.Sprintf("./testdata/uploads/%s", uploadedFiles.NewFileName)); os.IsNotExist(err) {
		t.Errorf("expected file to exist: %s", err.Error())
	}

	// clean up
	_ = os.Remove(fmt.Sprintf("./testdata/uploads/%s", uploadedFiles.NewFileName))
}

func TestTools_CreateDirIfNotExist(t *testing.T) {
	var testTool Tools

	err := testTool.CreateDirIfNotExist("./testdata/myDir")
	if err != nil {
		t.Error(err)
	}

	err = testTool.CreateDirIfNotExist("./testdata/myDir")
	if err != nil {
		t.Error(err)
	}
	_ = os.Remove("./testdata/myDir")
}

var slugTests = []struct {
	name          string
	s             string
	expected      string
	errorExpected bool
}{
	{name: "valid string", s: "now is the time", expected: "now-is-the-time", errorExpected: false},
	{name: "invalid slug", s: "now is the time", expected: "", errorExpected: true},
	{name: "empty string", s: "", expected: "", errorExpected: true},
	{name: "only asian characters", s: "こんにちは", expected: "", errorExpected: true},
	{name: "mixed roman, asian characters", s: "こんにちはhello World", expected: "hello-world", errorExpected: false},
}

func TestTools_Slugify(t *testing.T) {
	var testTool Tools

	for _, e := range slugTests {
		slugified, err := testTool.Slugify(e.s)
		if err != nil && !e.errorExpected {
			t.Errorf("test %s error received when none was expected: %s", e.name, err.Error())
		}
		if !e.errorExpected && slugified != e.expected {
			t.Errorf("test: %s failed to slugify string as expexted, wanted %s but got %s", e.name, e.expected, slugified)
		}
	}
}

func TestTools_DownloadStaticFile(t *testing.T) {
	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)

	var testTool Tools
	testTool.DownloadStaticFile(rr, req, "./testdata/img.png", "landscape.png")
	res := rr.Result()
	defer res.Body.Close()

	if res.Header["Content-Length"][0] != "534283" {
		t.Error("wrong content length of", res.Header["Content-Length"][0])
	}

	if res.Header["Content-Disposition"][0] != "attachment; filename=\"landscape.png\"" {
		t.Errorf("wrong content disposition: filename got %s but expected %s", res.Header["Content-Disposition"][0], "landscape.png")
	}

	_, err := io.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
}

var jsonTests = []struct {
	name               string
	json               string
	errorExpected      bool
	maxSize            int
	allowUnknownFields bool
}{
	// {name: "valid json", json: `{"foo": "bar"}`, errorExpected: false, maxSize: 1024, allowUnknownFields: false},
	// {name: "bad format", json: `{"foo": }`, errorExpected: true, maxSize: 1024, allowUnknownFields: false},
	// {name: "incorrect type", json: `{"foo": 10}`, errorExpected: true, maxSize: 1024, allowUnknownFields: false},
	// {name: "two json files", json: `{"foo": "bar"}{"second","file"}`, errorExpected: true, maxSize: 1024, allowUnknownFields: false},
	// {name: "file too big", json: `{"foo": "bar"}`, errorExpected: true, maxSize: 1, allowUnknownFields: false},
	// {name: "syntax error", json: `{"foo": ttt"}`, errorExpected: true, maxSize: 1024, allowUnknownFields: false},
	// {name: "empty json", json: `{"":""}`, errorExpected: true, maxSize: 1024, allowUnknownFields: false},
	// {name: "invalid field", json: `{"": "bar"}`, errorExpected: true, maxSize: 1024, allowUnknownFields: false},
	// {name: "unknown field but not allowed ", json: `{"fooo": "bar"}`, errorExpected: true, maxSize: 1024, allowUnknownFields: false},
	// {name: "unknown field allowed but error", json: `{"fooo": "bar"}`, errorExpected: false, maxSize: 1024, allowUnknownFields: true},
	// {name: "missing field name", json: `{fooo: "bar"}`, errorExpected: true, maxSize: 1024, allowUnknownFields: false},
	// {name: "not json", json: `Hello gorgeous`, errorExpected: true, maxSize: 1024, allowUnknownFields: true},

	{name: "good json", json: `{"foo": "bar"}`, errorExpected: false, maxSize: 1024, allowUnknownFields: false},
	{name: "badly formatted json", json: `{"foo":}`, errorExpected: true, maxSize: 1024, allowUnknownFields: false},
	{name: "incorrect type", json: `{"foo": 1}`, errorExpected: true, maxSize: 1024, allowUnknownFields: false},
	{name: "two json files", json: `{"foo": "1"}{"alpha": "beta"}`, errorExpected: true, maxSize: 1024, allowUnknownFields: false},
	{name: "empty body", json: ``, errorExpected: true, maxSize: 1024, allowUnknownFields: false},
	{name: "syntax error in json", json: `{"foo": 1"`, errorExpected: true, maxSize: 1024, allowUnknownFields: false},
	{name: "unknown field in json", json: `{"fooo": "1"}`, errorExpected: true, maxSize: 1024, allowUnknownFields: false},
	{name: "allow unknown fields in json", json: `{"fooo": "1"}`, errorExpected: false, maxSize: 1024, allowUnknownFields: true},
	{name: "missing field name", json: `{jack: "1"}`, errorExpected: true, maxSize: 1024, allowUnknownFields: true},
	{name: "file too large", json: `{"foo": "bar"}`, errorExpected: true, maxSize: 5, allowUnknownFields: true},
	{name: "not json", json: `Hello world!`, errorExpected: true, maxSize: 1024, allowUnknownFields: true},
}

func TestTools_ReadJSON(t *testing.T) {
	var testTool Tools

	for _, e := range jsonTests {
		testTool.MaxJSONSize = e.maxSize
		testTool.AllowUnknownFields = e.allowUnknownFields

		var decodedJSON struct {
			Foo string `json:"foo"`
		}
		req, err := http.NewRequest("POST", "/", bytes.NewReader(([]byte(e.json))))
		if err != nil {
			t.Log("Error:", err)
		}

		rr := httptest.NewRecorder()

		err = testTool.ReadJSON(rr, req, &decodedJSON)

		if e.errorExpected && err == nil {
			t.Errorf("test %s: error expected but none received", e.name)
		}

		if !e.errorExpected && err != nil {
			t.Errorf("test %s: error not expected but received one: %s", e.name, err.Error())
		}
		req.Body.Close()
	}
}

func TestTools_WriteJSON(t *testing.T) {
	var testTools Tools
	rr := httptest.NewRecorder()
	payload := JSONResponse{
		Error:   false,
		Message: "foo",
	}

	headers := make(http.Header)
	headers.Add("FOO", "BAR")

	err := testTools.WriteJSON(rr, http.StatusOK, payload, headers)
	if err != nil {
		t.Errorf("failed to write JSON: %v", err)
	}

}

func TestTools_ErrorJSON(t *testing.T) {

	var testTools Tools
	rr := httptest.NewRecorder()
	err := testTools.ErrorJSON(rr, errors.New("some error"), http.StatusServiceUnavailable)
	if err != nil {
		t.Error(err)
	}
	var payload JSONResponse
	decoder := json.NewDecoder(rr.Body)
	err = decoder.Decode(&payload)
	if err != nil {
		t.Error("received error while decoding JSON", err)
	}

	if !payload.Error {
		t.Error("error set to false in JSON, and it should be true")
	}

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("wrong status code returned; expected %d, but got %d", http.StatusServiceUnavailable, rr.Code)
	}
}
