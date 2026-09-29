package runtime

import (
	"reflect"
	"slices"
	"testing"
)

func TestTaskConfigCompareVolumes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		left  []string
		right []string
		equal bool
	}{
		{name: "omitted volumes", equal: true},
		{name: "nil and empty volumes", right: []string{}, equal: true},
		{name: "same volumes", left: []string{"data:/data"}, right: []string{"data:/data"}, equal: true},
		{name: "reordered volumes", left: []string{"data:/data", "/srv/config:/config:ro"}, right: []string{"/srv/config:/config:ro", "data:/data"}, equal: true},
		{name: "added volume", right: []string{"data:/data"}},
		{name: "removed volume", left: []string{"data:/data"}},
		{name: "changed source", left: []string{"data:/data"}, right: []string{"other:/data"}},
		{name: "changed target", left: []string{"data:/data"}, right: []string{"data:/other"}},
		{name: "changed options", left: []string{"data:/data"}, right: []string{"data:/data:ro"}},
		{name: "duplicate counts differ", left: []string{"data:/data", "data:/data", "logs:/logs"}, right: []string{"data:/data", "logs:/logs", "logs:/logs"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			left := TaskConfig{Name: "app", Image: "app:latest", Volumes: tc.left}
			right := TaskConfig{Name: "app", Image: "app:latest", Volumes: tc.right}
			leftVolumes := slices.Clone(tc.left)
			rightVolumes := slices.Clone(tc.right)
			if got := left.CompareTo(&right); got != tc.equal {
				t.Fatalf("CompareTo = %t, want %t", got, tc.equal)
			}
			if got := right.CompareTo(&left); got != tc.equal {
				t.Fatalf("reverse CompareTo = %t, want %t", got, tc.equal)
			}
			if !reflect.DeepEqual(left.Volumes, leftVolumes) || !reflect.DeepEqual(right.Volumes, rightVolumes) {
				t.Fatal("CompareTo changed the configured volume order")
			}
		})
	}
}
