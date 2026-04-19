package dashapp

// moveFocus picks the widget reachable in direction (dRow, dCol) that
// is nearest by Manhattan distance to the currently focused widget.
// If no widget exists in that direction, focus stays put.
func moveFocus(widgets []Widget, fromIdx, dRow, dCol int) int {
	if fromIdx < 0 || fromIdx >= len(widgets) {
		return fromIdx
	}
	curRow, curCol := widgets[fromIdx].Position()

	bestIdx := fromIdx
	bestDist := -1
	for i, w := range widgets {
		if i == fromIdx {
			continue
		}
		row, col := w.Position()
		switch {
		case dRow < 0 && row >= curRow:
			continue
		case dRow > 0 && row <= curRow:
			continue
		case dCol < 0 && col >= curCol:
			continue
		case dCol > 0 && col <= curCol:
			continue
		case dRow == 0 && dCol == 0:
			continue
		}
		dist := abs(row-curRow) + abs(col-curCol)
		if bestDist < 0 || dist < bestDist {
			bestDist = dist
			bestIdx = i
		}
	}
	return bestIdx
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
