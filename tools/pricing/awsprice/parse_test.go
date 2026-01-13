// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package awsprice

import "testing"

func TestParseMemory(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr bool
	}{
		{
			name: "valid GiB",
			args: args{
				s: "16 GiB",
			},
			want: 16 * 1024 * 1024 * 1024,
		},
		{
			name: "valid GB",
			args: args{
				s: "8 GB",
			},
			want: 8 * 1000 * 1000 * 1000,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMemory(tt.args.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMemory() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseMemory() got = %v, want %v", got, tt.want)
			}
		})
	}
}
