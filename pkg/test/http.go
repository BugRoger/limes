/*******************************************************************************
*
* Copyright 2017 Stefan Majewsky <majewsky@gmx.net>
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You should have received a copy of the License along with this
* program. If not, you may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*
*******************************************************************************/

package test

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"
)

//APIRequest contains all metadata about a test request.
type APIRequest struct {
	Method           string
	Path             string
	Body             string //may be empty
	ExpectStatusCode int
	ExpectJSON       string //path to JSON file
}

//Check performs the HTTP request described by this APIRequest against the
//given router and compares the response with the expectation in the
//APIRequest.
func (r APIRequest) Check(t *testing.T, router *mux.Router) {
	var requestBody io.Reader
	if r.Body != "" {
		requestBody = bytes.NewReader([]byte(r.Body))
	}
	request := httptest.NewRequest(r.Method, r.Path, requestBody)
	request.Header.Set("X-Auth-Token", "something")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	response := recorder.Result()
	responseBytes, _ := ioutil.ReadAll(response.Body)

	if response.StatusCode != r.ExpectStatusCode {
		t.Errorf("%s %s: expected status code %d, got %d",
			r.Method, r.Path, r.ExpectStatusCode, response.StatusCode,
		)
	}

	if r.ExpectJSON == "" {
		if len(responseBytes) != 0 {
			t.Fatalf("%s %s: expected no body, but got: %s",
				r.Method, r.Path, string(responseBytes),
			)
		}
		return //if no JSON response expected, don't try to parse the response body
	}

	var buf bytes.Buffer
	err := json.Indent(&buf, responseBytes, "", "  ")
	if err != nil {
		t.Logf("Response body: %s", responseBytes)
		t.Fatal(err)
	}
	buf.WriteByte('\n')

	fixturePath, _ := filepath.Abs(r.ExpectJSON)
	cmd := exec.Command("diff", "-u", fixturePath, "-")
	cmd.Stdin = &buf
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		t.Fatalf("%s %s: body does not match: %s", r.Method, r.Path, err.Error())
	}
}