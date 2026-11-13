package widgets

func Shrink(start, room float32, wide func(size float32) float32) float32 {
	size := max(start, 1)
	for range 8 {
		w := wide(size)
		if w <= room || size <= 1 {
			return size
		}
		size = max(min(size*room/w, size-1), 1)
	}
	return size
}
