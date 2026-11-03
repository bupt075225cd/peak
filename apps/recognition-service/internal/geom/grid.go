package geom

import "fmt"

// gridCoverTol 允许顶点落在网格边界外多少格（描点与格的量化误差）。
const gridCoverTol = 0.6

// CheckGridCoverage 网格覆盖核对：图中的顶点必须落在网格范围内。
// 点跑到网格之外，说明模型给的 cols/rows 或 cell 与它自己给的点坐标不自洽
// （常见于"为了把图形塞进网格而随便定行列数"），需要按原图网格边界重新数格线。
// 只提示一次，避免同一张图刷出多条相同问题。
func CheckGridCoverage(panels []Panel) {
	for i := range panels {
		spec := &panels[i].Spec
		g := spec.Grid
		if g == nil || g.Cell <= 0 || g.Cols <= 0 || g.Rows <= 0 {
			continue
		}
		used := figurePointNames(spec)
		for _, p := range spec.Points {
			if p.Hidden || !used[p.Name] {
				continue
			}
			fx := (p.X - g.X) / g.Cell
			fy := (p.Y - g.Y) / g.Cell
			if fx < -gridCoverTol || fx > float64(g.Cols)+gridCoverTol ||
				fy < -gridCoverTol || fy > float64(g.Rows)+gridCoverTol {
				panels[i].Issues = append(panels[i].Issues, fmt.Sprintf(
					"点 %s 落在网格范围之外（格坐标 %.1f,%.1f），说明网格的列数/行数或格边长估错了。"+
						"请按原图网格自身的边界重新数格子（横向间隔数=列数，纵向间隔数=行数），"+
						"并让每个顶点都落在网格内的格点上。", p.Name, fx, fy))
				break
			}
		}
	}
}
