/*
 * Copyright 2023 Hewlett Packard Enterprise Development LP
 * Other additional copyright holders may be indicated within.
 *
 * The entirety of this work is licensed under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 *
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package version

// The current data-mover version string. Clients can use this to identify particular
// information about the installed data-mover daemon.
var (
	// Version contains the current version of the data mover daemon.
	// This is a version tag.  If there are commits past that tag, then
	// a count is appended with a short git hash of the latest commit.
	version = "v0.0.0"

	// API Versions defines the list of API endpinds supported by the data-mover daemon.
	// If the protobuf API changes that is not backwards compatible, a new version should be introduced.
	//
	// All API endpoints are part of the same version, so a new CreateV2 endpoint would cause all endpoints
	// to transition to "V2" (StatusV2, DeleteV2, etc) even if no modifications are made.
	//
	// This value is an array in case a particular API introduced has a defect. For example, if a new API
	// is introduced "v3-beta", but found to be defective, that particular API can be removed and "v3" added
	// once the defect is resolved and the v3 API finalized.
	apiVersions = []string{"1"}
)

func BuildVersion() string { return version }

func ApiVersions() []string { return apiVersions }
