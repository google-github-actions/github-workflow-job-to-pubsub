// Copyright 2021 the GitHub Workflow Job to PubSub authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sethvargo/github-workflow-job-to-pubsub/internal/pubsub"
)

type jsonResponse struct {
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type Server struct {
	pubsubClient *pubsub.Client
}

func respondJSON(w http.ResponseWriter, i *jsonResponse, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if i != nil {
		b, err := json.Marshal(i)
		if err != nil {
			panic(err)
		}
		fmt.Fprint(w, string(b))
	}
}

// isValidSignature determines if the provided signature matches the expected
// signature.
func isValidSignature(want string, body []byte) bool {
	h := hmac.New(sha256.New, []byte(GitHubWebhookSecret))
	h.Write(body)
	got := GitHubSignaturePrefix + hex.EncodeToString(h.Sum(nil))

	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}
