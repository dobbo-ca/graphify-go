package util

type Calc struct{ total int }

func Add(a, b int) int { return a + b }

func (c *Calc) Sum(xs []int) int {
	for _, x := range xs {
		c.total = Add(c.total, x)
	}
	return c.total
}
