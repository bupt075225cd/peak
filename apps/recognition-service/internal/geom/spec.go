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

// Axis 是平面直角坐标系的一条轴：由原点指向正方向端点，末端绘制实心箭头。
type Axis struct {
	To    string `json:"to"`              // 正方向端点（须在 points 中定义，通常 hidden:true）
	Label string `json:"label,omitempty"` // 轴标签（如 "x"、"y"），缺省按 x/y 轴自动取
}

// Axes 是平面直角坐标系：原点 + 两条轴。
// 轴与矩形的边视觉上可能重合（如矩形 OABC 的 OA/OC 落在坐标轴上），
// 此时 axes 必须单独给出，且端点要落在图形轮廓之外，避免重绘退化成"一个矩形"。
// 注意：坐标轴不标注任何刻度数字——产品规则是"图上不标数值"，
// 已知长度/坐标只体现在图形与题干文字中；轴上只画 x / y 两个轴标签。
type Axes struct {
	Origin string `json:"origin"`
	X      Axis   `json:"x"`
	Y      Axis   `json:"y"`
}

// Curve 是函数图像/平滑曲线：按 through 中的点名顺序连接。
type Curve struct {
	Through []string `json:"through"`
	Smooth  bool     `json:"smooth,omitempty"`
	Dashed  bool     `json:"dashed,omitempty"`
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

// Grid 是小正方形网格背景（"在如图所示的网格中"这类题）。
// 用一块规则网格替代"几十个隐藏点 + 虚线线段"：模型只需给出左上角、格边长与行列数，
// 渲染器用浅灰细线补齐格线，格点坐标即 (X+i*Cell, Y+j*Cell)，A/B/C/D 等顶点应对齐格点。
type Grid struct {
	X      float64 `json:"x"` // 网格左上角
	Y      float64 `json:"y"`
	Cell   float64 `json:"cell"`             // 小正方形边长
	Cols   int     `json:"cols"`             // 列数
	Rows   int     `json:"rows"`             // 行数
	Dashed bool    `json:"dashed,omitempty"` // 原图网格为虚线时置 true
}

// Spec 是一张子图的几何描述。
//
// 注意：子图标题不在 Spec 内，而由 Panel（多子图包装）承载，
// 以免同一个 JSON 对象里出现两个含义不同的 title 字段。
type Spec struct {
	Canvas      Canvas         `json:"canvas"`
	Grid        *Grid          `json:"grid,omitempty"`
	Points      []Point        `json:"points"`
	Segments    []Segment      `json:"segments"`
	Axes        *Axes          `json:"axes,omitempty"`
	Curves      []Curve        `json:"curves,omitempty"`
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
