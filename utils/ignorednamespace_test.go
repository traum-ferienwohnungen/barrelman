package utils

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"k8s.io/utils/diff"
)

func Test_namespaceMap_Set(t *testing.T) {
	tests := []struct {
		name    string
		n       namespaceMap
		values  []string
		wantErr bool
		wantS   []string
	}{
		{
			"NoDuplicates",
			namespaceMap{"ignoreme": struct{}{}},
			[]string{"bla", "blub", "bla"},
			false,
			[]string{
				"ignoreme",
				"blub",
				"bla",
			},
		},
		{
			"Default",
			namespaceMap{"ignoreme": struct{}{}},
			[]string{},
			false,
			[]string{"ignoreme"},
		},
		{
			"Empty",
			namespaceMap{"ignoreme": struct{}{}},
			[]string{""},
			true,
			[]string{"ignoreme"},
		},
		{
			"Remove",
			namespaceMap{"ignoreme": struct{}{}},
			[]string{"-ignoreme", "-removeme", "expectme"},
			false,
			[]string{"expectme"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, v := range tt.values {
				if err := tt.n.Set(v); (err != nil) != tt.wantErr {
					t.Errorf("Set() error = %v, wantErr %v", err, tt.wantErr)
				}
			}
			gotS := strings.Split(tt.n.String(), ",")
			sort.Strings(gotS)
			sort.Strings(tt.wantS)
			if !reflect.DeepEqual(tt.wantS, gotS) {
				t.Errorf("Expected different namespaceMap: (expected, got)\n%s",
					diff.ObjectGoPrintSideBySide(tt.wantS, gotS))
			}
		})
	}
}
