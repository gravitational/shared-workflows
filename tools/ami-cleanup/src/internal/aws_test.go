/*
 * AMI cleanup tool
 * Copyright (C) 2024  Gravitational, Inc.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package internal

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
			t.Parallel()
			result := IsDryRunError(testCap.testError)
			require.Equal(t, testCap.expectedResult, result)
		})
	}
}
