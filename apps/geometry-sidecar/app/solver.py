# -*- coding: utf-8 -*-
"""几何约束求解器：输入“点集 + 约束列表 + 粗略锚点”，用最小二乘求出满足全部约束的坐标。

LLM 只负责产出约束 JSON，本模块是确定性计算，保证图形度量精确。

相比项目外 demo 的增强：
  - 支持圆相关约束：point_on_circle（点在圆上）、tangent（直线与圆相切）；
  - 多初始值策略：锚点、锚点扰动、水平/垂直镜像候选各求解一次，
    取硬约束残差最小者，避免最小二乘收敛到镜像/错误分支解；
  - 分层残差报告含逐约束明细（index/type/severity/residual），
    供上层把失败信息格式化后回喂 LLM 修正约束。

支持约束类型：
  horizontal      {"points": ["P","A","Q"]}                      所有点同一水平线
  point_on_line   {"point":"A","line":["P","Q"]}                 点在直线上（共线）
  angle           {"vertex":"D","a":"C","b":"A","deg":40}        ∠CDA = 40°（无向角）
  bisector        {"vertex":"A","side1":"P","side2":"B","mid":"C"}  AC 平分∠PAB
  parallel        {"p1":"A","p2":"B","q1":"C","q2":"D"}          AB ∥ CD
  between         {"point":"B","a":"C","b":"E"}                  B 在 C、E 之间（软约束）
  distance        {"a":"A","b":"B","val":1.5}                    |AB| = 1.5
  point_on_circle {"point":"A","center":"O","through":"B"}       A 在以 O 为圆心、过 B 的圆上
  tangent         {"line":["A","B"],"center":"O","through":"P"}  直线 AB 与圆(O,过P)相切

anchors 为各点粗略初始位置（来自 VLM 的目测即可），以小权重参与优化，
用于消除平移/旋转/缩放的自由度，不要求精确。
"""
import time

import numpy as np
from scipy.optimize import least_squares

# 通过/失败分两档：硬几何约束需 ~0；软约束(between/anchors)允许小误差。
HARD_TYPES = {"horizontal", "point_on_line", "angle", "bisector",
              "parallel", "distance", "point_on_circle", "tangent"}
GEOM_TOL = 2e-3      # 硬几何约束最大允许残差（角度约 0.1°，相对长度约 0.2%）
ANCHOR_TOL = 0.05    # 锚点/软约束允许偏差


class SolveError(ValueError):
    """约束不可求解（如未知约束类型、变量不足）。"""


# 允许出现在 constraints 中的类型之外的"绘图元素"类型：
# VLM 偶尔把绘图指令混进 constraints，这些不构成几何约束，直接丢弃。
_DRAWING_TYPES = {"segment", "line", "ray", "circle", "arc", "point"}

# 约束类型同义词映射（VLM 会发明近义类型名）。
_TYPE_SYNONYMS = {
    "point_between": "between",
    "betweenness": "between",
    "collinear": "point_on_line",
    "colinear": "point_on_line",
    "perpendicular": "angle",  # schema 兼容：vertex/a/b + deg=90
    "on_circle": "point_on_circle",
    "tangent_line": "tangent",
}


def normalize_constraints(constraints):
    """归一化 VLM 输出的约束列表：丢弃混入的绘图元素、映射同义类型。

    返回 (归一化后的约束列表, 丢弃说明列表)，丢弃说明用于日志/调试。
    """
    if not constraints:
        return [], []
    out, dropped = [], []
    for c in constraints:
        if not isinstance(c, dict):
            dropped.append(f"非对象约束: {c!r}")
            continue
        t = c.get("type")
        if t in _DRAWING_TYPES:
            dropped.append(f"绘图元素 {t} 混入 constraints（应放 elements），已忽略")
            continue
        if t in _TYPE_SYNONYMS:
            fixed = dict(c)
            fixed["type"] = _TYPE_SYNONYMS[t]
            if t == "perpendicular":
                fixed.setdefault("deg", 90)
            out.append(fixed)
            continue
        out.append(c)
    return out, dropped


def _hat(v):
    return v / (np.linalg.norm(v) + 1e-12)


def _cross2(a, b):
    return a[0] * b[1] - a[1] * b[0]


class GeoSolver:
    def __init__(self, points, constraints, anchors=None, anchor_weight=0.05):
        if not points:
            raise SolveError("points 为空，无法求解")
        self.names = list(points.keys())
        self.idx = {n: i for i, n in enumerate(self.names)}
        self.x0 = np.array([points[n] for n in self.names], float).ravel()
        self.cons = list(constraints or [])
        self.anchors = anchors or {}
        self.aw = anchor_weight

    def _P(self, x, name):
        i = self.idx.get(name)
        if i is None:
            raise SolveError(f"约束引用了未定义的点: {name}")
        return x[2 * i:2 * i + 2]

    # ---------------------------------------------------------------- 单条约束残差
    def _num(self, c, key):
        """从约束中读取数字参数（容忍 VLM 输出字符串数字），无效时报可读错误。"""
        v = c.get(key)
        if isinstance(v, bool):
            v = None
        elif isinstance(v, (int, float)):
            return float(v)
        elif isinstance(v, str):
            try:
                return float(v.strip())
            except ValueError:
                v = v  # 落入下方报错
        raise SolveError(
            f"约束 {c.get('type', '?')} 的参数 {key} 值无效: {v!r}（需为数字，如 40 或 1.5）"
        )

    def _constraint_residuals(self, x, c):
        """返回单条约束的残差列表。"""
        P = lambda n: self._P(x, n)
        t = c.get("type")
        if t == "horizontal":
            pts = [P(n) for n in c["points"]]
            return [a[1] - b[1] for a, b in zip(pts, pts[1:])]
        if t == "point_on_line":
            p1, p2, q = P(c["line"][0]), P(c["line"][1]), P(c["point"])
            d = p2 - p1
            return [_cross2(d, q - p1) / (np.linalg.norm(d) + 1e-9)]
        if t == "angle":
            v, pa, pb = P(c["vertex"]), P(c["a"]), P(c["b"])
            u, w = _hat(pa - v), _hat(pb - v)
            tau = np.radians(self._num(c, "deg"))
            cs, sn = np.dot(u, w), _cross2(u, w)
            # cos 与 sin² 两项：避免角度环绕问题，且对无向角鲁棒。
            return [cs - np.cos(tau), sn * sn - np.sin(tau) ** 2]
        if t == "bisector":
            v = P(c["vertex"])
            u, w = _hat(P(c["side1"]) - v), _hat(P(c["side2"]) - v)
            mm = _hat(P(c["mid"]) - v)
            return [_cross2(mm, u + w)]
        if t == "parallel":
            d1 = P(c["p2"]) - P(c["p1"])
            d2 = P(c["q2"]) - P(c["q1"])
            return [_cross2(d1, d2) / (np.linalg.norm(d1) * np.linalg.norm(d2) + 1e-9)]
        if t == "between":
            a, b, q = P(c["a"]), P(c["b"]), P(c["point"])
            d = b - a
            tpar = np.dot(q - a, d) / np.dot(d, d)
            return [_cross2(d, q - a) / (np.linalg.norm(d) + 1e-9),
                    1.0 * max(0.0, 0.08 - tpar), 1.0 * max(0.0, tpar - 0.92)]
        if t == "distance":
            return [float(np.linalg.norm(P(c["a"]) - P(c["b"])) - self._num(c, "val"))]
        if t == "point_on_circle":
            # |point - center| = |through - center|
            po = P(c["point"]) - P(c["center"])
            to = P(c["through"]) - P(c["center"])
            return [float(np.linalg.norm(po) - np.linalg.norm(to))]
        if t == "tangent":
            # 圆心到直线的距离 = 半径（半径由圆上一点 through 确定）。
            a, b, o = P(c["line"][0]), P(c["line"][1]), P(c["center"])
            d = b - a
            dist = abs(_cross2(d, o - a)) / (np.linalg.norm(d) + 1e-9)
            r = np.linalg.norm(P(c["through"]) - o)
            return [float(dist - r)]
        raise SolveError(f"未知约束类型: {t}")

    # ---------------------------------------------------------------- 全量残差
    def _residuals(self, x):
        """返回 (硬+软 拼接残差, 硬残差条数)，便于分层判定。"""
        hard, soft = [], []
        for c in self.cons:
            rs = self._constraint_residuals(x, c)
            (hard if c.get("type") in HARD_TYPES else soft).extend(rs)
        for n, pos in self.anchors.items():
            soft.extend(self.aw * (self._P(x, n) - np.array(pos, float)))
        return np.array(hard + soft, float), len(hard)

    # ---------------------------------------------------------------- 多初始值
    def _initial_candidates(self):
        """初始值候选：原锚点、扰动 ×2、水平/垂直/双向镜像。

        镜像候选用于对抗无向角（sin² 项）与弱锚点下最小二乘收敛到镜像分支的问题。
        """
        n = len(self.names)
        pts = self.x0.reshape(n, 2)
        center = pts.mean(axis=0)
        cands = [self.x0.copy()]
        scale = max(float(np.ptp(pts[:, 0])), float(np.ptp(pts[:, 1])), 0.1)
        rng = np.random.default_rng(42)
        for _ in range(2):
            cands.append(self.x0 + rng.normal(0, 0.1 * scale, self.x0.shape))
        flip_x = pts.copy(); flip_x[:, 0] = 2 * center[0] - flip_x[:, 0]
        flip_y = pts.copy(); flip_y[:, 1] = 2 * center[1] - flip_y[:, 1]
        flip_xy = pts.copy(); flip_xy[:, 0] = 2 * center[0] - flip_xy[:, 0]; flip_xy[:, 1] = 2 * center[1] - flip_xy[:, 1]
        for fp in (flip_x, flip_y, flip_xy):
            cands.append(fp.ravel())
        return cands

    # ---------------------------------------------------------------- 求解
    def solve(self, time_budget=20.0):
        """返回 (坐标 dict, 报告 dict)。

        time_budget 为总求解时间预算（秒）：病态/矛盾约束会让最小二乘迭代很久，
        超出预算后放弃剩余初始值候选，用当前最优结果返回（残差会体现在报告中，
        由上层回喂 VLM 修正约束）。

        report 字段：
          max_hard        硬几何约束最大残差（~0 即几何成立）
          max_soft        软约束（between/锚点）最大残差
          max_all         全部残差最大值
          per_constraint  逐约束明细 [{index,type,severity,residual}]
        """
        deadline = time.monotonic() + time_budget
        best = None  # (max_hard, max_soft, x, res, n_hard)
        last_err = None
        for i, x0 in enumerate(self._initial_candidates()):
            if i > 0 and time.monotonic() > deadline:
                break
            try:
                r = least_squares(lambda x: self._residuals(x)[0], x0,
                                  method="lm", max_nfev=20000)
            except SolveError:
                # 参数校验错误（无效 deg/val、未知点等）与初始值无关，快速失败。
                raise
            except Exception as e:  # LM 要求残差数 >= 变量数等场景，回退 TRF。
                last_err = e
                try:
                    r = least_squares(lambda x: self._residuals(x)[0], x0,
                                      method="trf", max_nfev=20000)
                except Exception as e2:
                    last_err = e2
                    continue
            x = r.x
            res, n_hard = self._residuals(x)
            hard = np.abs(res[:n_hard]) if n_hard else np.zeros(1)
            soft = np.abs(res[n_hard:])
            max_hard = float(hard.max()) if len(hard) else 0.0
            max_soft = float(soft.max()) if len(soft) else 0.0
            score = (round(max_hard, 12), round(max_soft, 12))
            if best is None or score < best[0]:
                best = (score, x, res, n_hard)
            # 已达几何自洽时无需继续尝试其他初始值。
            if best is not None and best[0][0] < GEOM_TOL:
                break
        if best is None:
            raise SolveError(f"所有初始值求解均失败: {last_err}")

        _, x, res, n_hard = best
        coords = {n: [float(x[2 * i]), float(x[2 * i + 1])] for i, n in enumerate(self.names)}
        per = []
        for i, c in enumerate(self.cons):
            try:
                rs = np.abs(np.array(self._constraint_residuals(x, c), float))
                residual = float(rs.max()) if len(rs) else 0.0
            except SolveError:
                raise
            per.append({"index": i, "type": c.get("type", "?"),
                        "severity": "hard" if c.get("type") in HARD_TYPES else "soft",
                        "residual": residual})
        hard_res = np.abs(res[:n_hard]) if n_hard else np.zeros(1)
        soft_res = np.abs(res[n_hard:])
        report = {
            "max_hard": float(hard_res.max()) if len(hard_res) else 0.0,
            "max_soft": float(soft_res.max()) if len(soft_res) else 0.0,
            "max_all": float(np.abs(res).max()) if len(res) else 0.0,
            "per_constraint": per,
        }
        return coords, report


def prepare_points(spec):
    """从 spec JSON 组装初始点集。anchors 必须提供（多初始值策略依赖它）。"""
    anchors = spec.get("anchors") or {}
    points = {n: list(np.array(a, float)) for n, a in anchors.items()}
    for n in spec.get("points") or []:
        points.setdefault(n, [0.0, 0.0])
    if not points:
        raise SolveError("spec 缺少 anchors/points，无法求解")
    return points
