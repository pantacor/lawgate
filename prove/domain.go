package prove

import "iter"

// Ints yields lo..hi inclusive, and nothing when lo > hi. It stops on hi
// before incrementing, so hi may be math.MaxInt without wrapping.
func Ints(lo, hi int) iter.Seq[int] {
	return func(yield func(int) bool) {
		if lo > hi {
			return
		}
		for i := lo; yield(i); i++ {
			if i == hi {
				return
			}
		}
	}
}

// Of yields the given values.
func Of[T any](values ...T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, value := range values {
			if !yield(value) {
				return
			}
		}
	}
}

// Concat yields every value of each sequence in turn.
func Concat[T any](seqs ...iter.Seq[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, seq := range seqs {
			if !yieldAll(seq, yield) {
				return
			}
		}
	}
}

// yieldAll passes every value of seq to yield and reports whether the
// consumer wants more.
func yieldAll[T any](seq iter.Seq[T], yield func(T) bool) bool {
	for value := range seq {
		if !yield(value) {
			return false
		}
	}
	return true
}

// Pair is one element of a product.
type Pair[A, B any] struct {
	A A
	B B
}

// Product yields every pair of a and b.
func Product[A, B any](a iter.Seq[A], b iter.Seq[B]) iter.Seq[Pair[A, B]] {
	return func(yield func(Pair[A, B]) bool) {
		for x := range a {
			pairWith := func(y B) bool { return yield(Pair[A, B]{A: x, B: y}) }
			if !yieldAll(b, pairWith) {
				return
			}
		}
	}
}
