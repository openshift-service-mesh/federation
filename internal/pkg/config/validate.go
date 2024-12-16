// Copyright Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the License);
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an AS IS BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"errors"
	"fmt"
)

func ValidateRemotes(remotes ...Remote) error {
	var err []error
	for _, remote := range remotes {
		err = append(err, ValidateRemote(remote))
	}

	return errors.Join(err...)
}

func ValidateRemote(remote Remote) error {
	if len(remote.Addresses) == 0 {
		return fmt.Errorf("remote [%s] should define at least one address", remote.Name)
	}

	return nil
}
