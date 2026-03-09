// MIT 许可证
//
// 版权所有 (c) 2016-2017 xtaci
//
// 特此免费授予任何获得本软件及相关文档文件（"软件"）副本的人不受限制地处理
// 本软件的权利，包括但不限于使用、复制、修改、合并、发布、分发、再许可和/或出售
// 本软件副本的权利，以及允许获得本软件的人员这样做，但须符合以下条件：
//
// 上述版权声明和本许可声明应包含在本软件的所有
// 副本或主要部分中。
//
// 本软件按"原样"提供，不提供任何形式的明示或
// 暗示担保，包括但不限于适销性、特定用途适用性和非侵权性担保。在任何情况下，
// 作者或版权持有人均不对任何索赔、损害或其他
// 责任负责，无论是在合同诉讼、侵权诉讼或其他诉讼中，均由软件或软件的使用或其他
// 软件交易引起或与之相关。

package smux

// _itimediff returns the time difference between two uint32 values.
// 结果为有符号 32 位整数，表示 'later' 与 'earlier' 之差。
func _itimediff(later, earlier uint32) int32 {
	return (int32)(later - earlier)
}

// shaperHeap is a min-heap of writeRequest.
// 按 class 优先排序写请求，同 class 内再按序列号排序。
type shaperHeap []writeRequest

func (h shaperHeap) Len() int { return len(h) }

// Less 决定堆中元素的排序顺序。
// 请求首先按 class 排序，若两个请求 class 相同，
// they are ordered by their sequence numbers.
func (h shaperHeap) Less(i, j int) bool {
	if h[i].class != h[j].class {
		return h[i].class < h[j].class
	}
	return _itimediff(h[j].seq, h[i].seq) > 0
}

func (h shaperHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *shaperHeap) Push(x interface{}) { *h = append(*h, x.(writeRequest)) }

func (h *shaperHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
