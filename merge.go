package tsl

import (
	"fmt"
)

// disjointRanges represents a slice of SortedRanges which do not
// overlap. Sorted iteration across disjoint ranges is quicker than
// iteration across overlapping ranges since it avoids the need
// for a comparison in order to select the cursor to read from next.
type disjointRanges struct {
	first    Element
	last     Element
	segments []SortedRange
}

func (d *disjointRanges) First() Element {
	return d.first
}

func (d *disjointRanges) Last() Element {
	return d.last
}

func (d *disjointRanges) Open() Cursor {
	return &disjointCursor{
		next:     0,
		cursor:   d.segments[0].Open(),
		segments: d.segments,
	}
}

func (d *disjointRanges) Limit() int {
	limit := 0
	for _, r := range d.segments {
		limit += r.Limit()
	}
	return limit
}

func (d *disjointRanges) Partition(e Element, o Order) (SortedRange, SortedRange) {
	if !o(d.first, e) {
		return EmptyRange, d
	}

	for i, r := range d.segments {
		if r.First() == nil {
			panic("r.First() is nil!")
		}
		if r.Last() == nil {
			panic("r.Last() is nil!")
		}
		if o(e, r.First()) && !o(r.First(), e) {
			r1, r2 := &disjointRanges{
				first:    d.first,
				last:     d.segments[i-1].Last(),
				segments: d.segments[0:i],
			}, &disjointRanges{
				first:    r.First(),
				last:     d.last,
				segments: d.segments[i:],
			}
			return r1, r2
		}
		if o(r.First(), e) && o(e, r.Last()) {
			p1, p2 := r.Partition(e, o)
			var r1, r2 SortedRange
			if p2.Limit() == 0 {
				if p1.Last() == nil {
					panic("p1.Last() is nil!")
				}
				r1 = &disjointRanges{
					first:    d.first,
					last:     p1.Last(),
					segments: append(d.segments[0:i], p1),
				}
				if i+1 < len(d.segments) {
					r2 = &disjointRanges{
						first:    d.segments[i+1].First(),
						last:     d.last,
						segments: d.segments[i+1:],
					}
				} else {
					r2 = EmptyRange
				}
			} else {
				if p1.Last() == nil {
					panic("p1.Last() is nil!")
				}
				if p2.First() == nil {
					panic("p2.First() is nil!")
				}
				r1, r2 = &disjointRanges{
					first:    d.first,
					last:     p1.Last(),
					segments: append(d.segments[0:i], p1),
				}, &disjointRanges{
					first:    p2.First(),
					last:     d.last,
					segments: append([]SortedRange{p2}, d.segments[i+1:]...),
				}
			}
			return r1, r2
		}
	}

	return d, EmptyRange
}

type disjointCursor struct {
	next     int
	cursor   Cursor
	segments []SortedRange
}

func (c *disjointCursor) nextCursor() Cursor {
	c.next++
	if c.next < len(c.segments) {
		return c.segments[c.next].Open()
	} else {
		return nil
	}
}

func (c *disjointCursor) Next() Element {
	var next Element
	for c.cursor != nil {
		next = c.cursor.Next()
		if next == nil {
			c.cursor = c.nextCursor()
		} else {
			break
		}
	}
	return next
}

func (c *disjointCursor) Fill(buffer []Element) int {
	max := len(buffer)
	next := 0
	for next < max && c.cursor != nil {
		filled := c.cursor.Fill(buffer[next:max])
		next += filled
		if next < max {
			c.cursor = c.nextCursor()
		}
	}
	return next
}

// selectFirst Choose r = a.First() or r = b.First() such that
// !b.First().Less(r) && !a.First().Less(r).
//
// If both a.First() and b.First() satisfy these constraints, then choose b.First()
func selectFirst(a, b Range) Element {
	if a.First() == nil {
		return b.First()
	} else if b.First() == nil {
		return a.First()
	} else if a.First().Less(b.First()) {
		return a.First()
	}
	return b.First()
}

// selectLast Choose r = a.Last() or r = b.Last() such that
// !r.Last().Less(a.Last()) && !r.Last().Less(b.Last()).
//
// If both a.Last() and b.Last() satisfy those constraints, then choose b.Last()
func selectLast(a, b Range) Element {
	if a.Last() == nil {
		return b.Last()
	} else if b.Last() == nil {
		return a.Last()
	} else if !b.Last().Less(a.Last()) {
		return b.Last()
	}
	return a.Last()
}

// flatten merges the segments slices of adjacent disjointRanges into a larger
// slice
func flatten(segments []SortedRange) []SortedRange {
	if len(segments) == 0 {
		return segments
	} else if _, ok := segments[0].(*disjointRanges); !ok {
		return append([]SortedRange{segments[0]}, flatten(segments[1:])...)
	} else {
		tmp := []SortedRange{}
		for i, r := range segments {
			if d, ok := r.(*disjointRanges); ok {
				tmp = append(tmp, d.segments...)
			} else {
				return append(tmp, flatten(segments[i:])...)
			}
		}
		return tmp
	}

}

func merge(a SortedRange, b SortedRange) SortedRange {
	if a.Limit() == 0 {
		return b
	} else if b.Limit() == 0 {
		return a
	} else if a.Last().Less(b.First()) {
		return &disjointRanges{
			first:    a.First(),
			last:     selectLast(a, b),
			segments: []SortedRange{a, b},
		}
	} else {
		first := selectFirst(a, b)
		last := selectLast(a, b)

		p1, p2 := a.Partition(b.First(), LessOrder)
		p3, p4 := b.Partition(a.Last(), LessOrEqualOrder)

		var m23 SortedRange

		if p2.Limit() == 0 {
			m23 = p3
		} else if p3.Limit() == 0 {
			m23 = p2
		} else {
			m23 = useEmptyRangeIfEmpty(newMergeableRange(selectFirst(p2, p3), selectLast(p2, p3), p2, p3, nil))
		}

		segments := []SortedRange{p1, m23, p4}
		j := 0
		for _, r := range segments {
			if r.Limit() > 0 {
				segments[j] = r
				j++
			}
		}
		segments = segments[0:j]

		switch len(segments) {
		case 0:
			panic("unexpected")
		case 1:
			return segments[0]
		default:
			return &disjointRanges{
				first:    first,
				last:     last,
				segments: flatten(segments),
			}
		}
	}
}

func (d *disjointRanges) String() string {
	buf := fmt.Sprintf("disjointRanges{first: %v, last: %v, segments: [", d.first, d.last)
	for i, s := range d.segments {
		if i > 0 {
			buf = buf + ","
		}
		buf = buf + fmt.Sprintf("%v", s)
	}
	buf = buf + "]}"
	return buf
}
