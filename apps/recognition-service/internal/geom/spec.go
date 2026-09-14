// Package geom 定义"供程序重新绘制"的结构化几何描述，并把描述渲染为独立 SVG。
//
// 与旧方案（外部服务做约束求解 + matplotlib 渲染）不同，本包不做任何数值求解：
// 模型直接给出各点坐标与图元结构，这里只负责清洗、结构校验、文字避让与 SVG 生成。
// 因此图形度量不保证数学精确，以"比例协调、不违背题意"为准。
//
// 坐标系与 SVG 保持一致：原点在画布左上角，x 轴向右、y 轴向下，
// 所有坐标都归一化到 Canvas 描述的范围内。
package geom

const (
	// DefaultCanvasSize 是缺省画布边长。
	DefaultCanvasSize = 100.0
	// maxRadiusRatio 限制圆/弧半径相对画布的比例，过滤模型的明显错误数值。
	maxRadiusRatio = 2.0
)

// Canvas 描述归一化画布尺寸。
type Canvas struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Point 是图中的点（顶点、圆心、交点等）。
type Point struct {
	Name    string   `json:"name"`
	X       float64  `json:"x"`
	Y       float64  `json:"y"`
	Label   string   `json:"label,omitempty"`    // 展示用标签，缺省用 Name
	LabelDX *float64 `json:"label_dx,omitempty"` // 标签相对点的偏移，缺省按图形中心自动外移
	LabelDY *float64 `json:"label_dy,omitempty"`
	Hidden  bool     `json:"hidden,omitempty"` // 只作构造点，不绘制实心点
}

// Segment 是线段；Extend 为真时表示向两端延长的直线/射线。
type Segment struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Dashed bool   `json:"dashed,omitempty"`
	Extend bool   `json:"extend,omitempty"` // 表示延长线
}

// Polygon 是多边形，可按需填充。
type Polygon struct {
	Points []string `json:"points"`
	Fill   bool     `json:"fill,omitempty"`
	Dashed bool     `json:"dashed,omitempty"`
}

// Circle 是圆，Radius 与 Through 二者取一。
type Circle struct {
	Center  string  `json:"center"`
	Radius  float64 `json:"radius,omitempty"`
	Through string  `json:"through,omitempty"`
	Dashed  bool    `json:"dashed,omitempty"`
}

// Arc 是圆弧。角度单位为度：0° 指向 x 轴正方向，角度增大方向与 y 轴正向（向下）一致。
type Arc struct {
	Center     string  `json:"center"`
	Radius     float64 `json:"radius,omitempty"`
	StartAngle float64 `json:"start_angle"`
	EndAngle   float64 `json:"end_angle"`
	Dashed     bool    `json:"dashed,omitempty"`
}

// RightAngle 是直角标记。
type RightAngle struct {
	Vertex string  `json:"vertex"`
	A      string  `json:"a"`
	B      string  `json:"b"`
	Size   float64 `json:"size,omitempty"`
}

// AngleMark 是角的弧线标记。按产品要求图上不标注任何角度文字，
// 因此 Label 字段一律被忽略（保留仅为兼容模型可能的多余输出）。
type AngleMark struct {
	Vertex string  `json:"vertex"`
	A      string  `json:"a"`
	B      string  `json:"b"`
	Radius float64 `json:"radius,omitempty"`
	Label  string  `json:"label,omitempty"`
	Count  int     `json:"count,omitempty"` // 弧线条数 1~3
}

// TickMark 是线段上的等长标记。
type TickMark struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count,omitempty"`
}

// ParallelMark 是线段上的平行标记。
type ParallelMark struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count,omitempty"`
}

// Label 是自由文本标注（边长、代数式、结论等）。
type Label struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Text   string  `json:"text"`
	Anchor string  `json:"anchor,omitempty"` // start | middle | end
	Size   float64 `json:"size,omitempty"`
}

// Spec 是一张子图的几何描述。
//
// 注意：子图标题不在 Spec 内，而由 Panel（多子图包装）承载，
// 以免同一个 JSON 对象里出现两个含义不同的 title 字段。
type Spec struct {
	Canvas      Canvas         `json:"canvas"`
	Points      []Point        `json:"points"`
	Segments    []Segment      `json:"segments"`
	Polygons    []Polygon      `json:"polygons,omitempty"`
	Circles     []Circle       `json:"circles,omitempty"`
	Arcs        []Arc          `json:"arcs,omitempty"`
	RightAngles []RightAngle   `json:"right_angles,omitempty"`
	AngleMarks  []AngleMark    `json:"angle_marks,omitempty"`
	Ticks       []TickMark     `json:"ticks,omitempty"`
	Parallels   []ParallelMark `json:"parallels,omitempty"`
	Labels      []Label        `json:"labels,omitempty"`
	SVG         string         `json:"svg,omitempty"` // 兜底：模型直接给出的完整 SVG
}
