// Copyright 2017 Hewlett Packard Enterprise Development LP
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//    License for the specific language governing permissions and limitations
//    under the License.

package aggregation

func CheckSubArray(subArray []string, array []string) (isSubArray bool) {
	// empty array is a sub array of all arrays
	if len(subArray) == 0 {
		return true
	}
	for _, subElement := range subArray {
		isInArray := CheckInArray(subElement, array)
		if !isInArray {
			return false
		}
	}
	return true
}

func CheckInArray(str string, array[]string) (isInArray bool) {
	// Check if individual string is in target array
	for _, strArray := range array {
		if str == strArray {
			return true
		}
	}
	return false
}
