// Copyright © 2020 The Knative Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package functions

type FunkConfig struct {
	Functions FunkList `json:"funks"`
}

type FunkList []FunkFunction

type FunkFunction struct {
	Name    string `json:"name"`
	Source  string `json:"source"`
	Type    string `json:"type"`
	Returns string `json:"return-type"`
}
