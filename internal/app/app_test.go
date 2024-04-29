package app

import (
	"reflect"
	"testing"
)

func TestPrepend(t *testing.T) {
	tests := []struct {
		name     string
		slice    interface{}
		item     interface{}
		expected interface{}
	}{
		{
			name:     "Integers",
			slice:    []int{2, 3, 4},
			item:     1,
			expected: []int{1, 2, 3, 4},
		},
		{
			name:     "Strings",
			slice:    []string{"b", "c", "d"},
			item:     "a",
			expected: []string{"a", "b", "c", "d"},
		},
		{
			name:     "Floats",
			slice:    []float64{2.0, 3.0, 4.0},
			item:     1.0,
			expected: []float64{1.0, 2.0, 3.0, 4.0},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var result interface{}

			switch slice := tc.slice.(type) {
			case []int:
				item := tc.item.(int)
				result = prepend(slice, item)
			case []string:
				item := tc.item.(string)
				result = prepend(slice, item)
			case []float64:
				item := tc.item.(float64)
				result = prepend(slice, item)
			default:
				t.Fatalf("Unsupported type in test case")
			}

			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Test %s failed: expected %v, got %v", tc.name, tc.expected, result)
			}
		})
	}
}
func BenchmarkPrepend(b *testing.B) {
	benchmarks := []struct {
		name  string
		slice interface{}
		item  interface{}
	}{
		{
			name:  "Integers",
			slice: []int{2, 3, 4},
			item:  1,
		},
		{
			name:  "Strings",
			slice: []string{"b", "c", "d"},
			item:  "a",
		},
		{
			name:  "Floats",
			slice: []float64{2.0, 3.0, 4.0},
			item:  1.0,
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			var result interface{}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				switch slice := bm.slice.(type) {
				case []int:
					item := bm.item.(int)
					result = prepend(slice, item)
				case []string:
					item := bm.item.(string)
					result = prepend(slice, item)
				case []float64:
					item := bm.item.(float64)
					result = prepend(slice, item)
				default:
					b.Fatalf("Unsupported type in benchmark case")
				}
				_ = result
			}
		})
	}
}
