/*
 * Copyright 2024 Gravitational, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
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

package cleanup

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/stretchr/testify/require"
)

func TestIsDryRunError(t *testing.T) {
	tests := []struct {
		desc           string
		testError      error
		expectedResult bool
	}{
		{
			desc:           "no match if nil",
			testError:      nil,
			expectedResult: false,
		},
		{
			desc:           "no match on generic error",
			testError:      errors.New(""),
			expectedResult: false,
		},
		{
			desc: "match when valid dry run error",
			testError: &smithy.OperationError{
				Err: &http.ResponseError{
					ResponseError: &smithyhttp.ResponseError{
						Err: &smithy.GenericAPIError{
							Code: "DryRunOperation",
						},
					},
				},
			},
			expectedResult: true,
		},
	}

	for _, test := range tests {
		testCap := test // Capture the loop var, unnecessary in upcoming go 1.22
		t.Run(test.desc, func(t *testing.T) {
			result := IsDryRunError(testCap.testError)
			require.Equal(t, testCap.expectedResult, result)
		})
	}
}
