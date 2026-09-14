# -*- coding: utf-8 -*-
"""求解器与渲染器测试。

用 demo 实测样例（fig1/fig3）验证求解精度，另构造圆/切线用例验证新增约束，
渲染做冒烟验证（SVG 输出合法、含中文 title 时不出错）。
"""
import json
import os

import pytest

from app.renderer import render
from app.solver import ANCHOR_TOL, GEOM_TOL, GeoSolver, SolveError, prepare_points, normalize_constraints

DATA = os.path.join(os.path.dirname(__file__), "data")


def _load(name):
    with open(os.path.join(DATA, name), encoding="utf-8") as f:
        return json.load(f)


def _solve(spec):
    points = prepare_points(spec)
    return GeoSolver(points, spec["constraints"], anchors=spec.get("anchors", {})).solve()


class TestDemoSpecs:
    """demo 实测样例：硬约束残差应远小于 GEOM_TOL。"""

    @pytest.mark.parametrize("name", ["fig1.json", "fig3.json"])
    def test_solve_precision(self, name):
        spec = _load(name)
        coords, report = _solve(spec)
        assert report["max_hard"] < GEOM_TOL
        assert report["max_soft"] < ANCHOR_TOL
        assert set(coords) >= set(spec["points"])

    def test_fig1_angle_constraints_hold(self):
        """fig1 的核心几何关系：AC 平分 ∠PAB（∠PAC=∠CAB=50°）、∠BAD=40°。"""
        spec = _load("fig1.json")
        coords, _ = _solve(spec)

        def ang(v, a, b):
            import numpy as np
            v, a, b = np.array(coords[v]), np.array(coords[a]), np.array(coords[b])
            u, w = a - v, b - v
            cos = np.dot(u, w) / (np.linalg.norm(u) * np.linalg.norm(w))
            return float(np.degrees(np.arccos(np.clip(cos, -1, 1))))

        assert abs(ang("A", "P", "C") - 50) < 0.1
        assert abs(ang("A", "C", "B") - 50) < 0.1
        assert abs(ang("A", "B", "D") - 40) < 0.1


class TestCircleConstraints:
    def test_point_on_circle_and_tangent(self):
        """圆心 O、半径 1（过 A）：B 在圆上、直线 T1T2 在 A 处与圆相切。"""
        spec = {
            "points": ["O", "A", "B", "T1", "T2"],
            "anchors": {"O": [0, 0], "A": [1, 0], "B": [0, 1],
                        "T1": [1, -0.5], "T2": [1, 0.5]},
            "constraints": [
                {"type": "distance", "a": "O", "b": "A", "val": 1.0},
                {"type": "point_on_circle", "point": "B", "center": "O", "through": "A"},
                {"type": "tangent", "line": ["T1", "T2"], "center": "O", "through": "A"},
                # T1/T2 需要在切线上且 A 介于其间，保证切点确实落在直线上。
                {"type": "point_on_line", "point": "A", "line": ["T1", "T2"]},
                {"type": "between", "point": "A", "a": "T1", "b": "T2"},
            ],
        }
        coords, report = _solve(spec)
        assert report["max_hard"] < GEOM_TOL

        import numpy as np
        O, A, B = (np.array(coords[n]) for n in ("O", "A", "B"))
        assert abs(np.linalg.norm(A - O) - 1.0) < 1e-3   # 半径 1
        assert abs(np.linalg.norm(B - O) - 1.0) < 1e-3   # B 在圆上
        # 半径 OA 与切线 T1T2 垂直。
        tangent = np.array(coords["T2"]) - np.array(coords["T1"])
        assert abs(float(np.dot(tangent, A - O))) < 1e-2

    def test_auto_point_without_point_elements(self):
        """elements 不含 point 元素时，自动为 points 中所有顶点补字母标注（不崩溃）。"""
        coords = {"O": [0.0, 0.0], "A": [1.0, 0.0], "B": [0.0, 1.0], "C": [1.0, 1.0]}
        spec = {
            "points": ["O", "A", "B", "C"],
            "anchors": coords,
            "elements": [
                {"type": "segment", "pts": ["A", "B"]},
                {"type": "segment", "pts": ["B", "C"]},
                {"type": "segment", "pts": ["C", "O"]},
            ],
        }
        svg = render(spec, coords).decode("utf-8")
        assert "<svg" in svg
        # 自动补出的顶点字母应出现。
        assert "A" in svg and "B" in svg and "O" in svg

    def test_circle_and_arc_render(self):
        coords = {"O": [0.0, 0.0], "A": [1.0, 0.0], "B": [0.0, 1.0]}
        spec = {
            "title": "圆与弧测试",
            "elements": [
                {"type": "circle", "center": "O", "through": "A"},
                {"type": "arc", "center": "O", "a": "A", "b": "B", "color": "blue"},
                {"type": "segment", "pts": ["O", "A"]},
                {"type": "point", "name": "O", "label_offset": [0.0, -0.1]},
                {"type": "point", "name": "A"},
                {"type": "point", "name": "B"},
            ],
            "annotations": [
                {"type": "angle_mark", "vertex": "O", "from": "A", "to": "B",
                 "label": "90°", "color": "blue"},
            ],
        }
        svg = render(spec, coords).decode("utf-8")
        assert svg.lstrip().startswith("<?xml") or "<svg" in svg


class TestRobustness:
    def test_mirror_candidates_recover_solution(self):
        """锚点整体水平镜像（手性相反）时，多初始值策略应仍收敛到正确解。"""
        base = _load("fig1.json")
        coords, report = _solve(base)
        assert report["max_hard"] < GEOM_TOL

        mirrored = json.loads(json.dumps(base))
        xs = [a[0] for a in mirrored["anchors"].values()]
        cx = (min(xs) + max(xs)) / 2
        for a in mirrored["anchors"].values():
            a[0] = 2 * cx - a[0]
        mcoords, mreport = _solve(mirrored)
        assert mreport["max_hard"] < GEOM_TOL
        # 两解的角关系一致（手性不同但几何自洽等价）。
        import numpy as np
        d1 = np.array(coords["A"]) - np.array(coords["B"])
        d2 = np.array(mcoords["A"]) - np.array(mcoords["B"])
        assert abs(np.linalg.norm(d1) - np.linalg.norm(d2)) < 0.3

    def test_unknown_point_raises(self):
        spec = {"points": ["A"], "anchors": {"A": [0, 0]},
                "constraints": [{"type": "distance", "a": "A", "b": "X", "val": 1}]}
        with pytest.raises(SolveError):
            _solve(spec)

    def test_unknown_constraint_type_raises(self):
        spec = {"points": ["A", "B"], "anchors": {"A": [0, 0], "B": [1, 0]},
                "constraints": [{"type": "frobnicate"}]}
        with pytest.raises(SolveError):
            _solve(spec)

    def test_render_adaptive_large_figure(self):
        """坐标范围大时 label_offset 自适应缩放，渲染不报错。"""
        coords = {n: [i * 100.0, 0.0] for i, n in enumerate("ABCDE")}
        spec = {"elements": [{"type": "point", "name": n, "label_offset": [0.0, 0.1]}
                             for n in coords]}
        svg = render(spec, coords).decode("utf-8")
        assert "<svg" in svg


class TestSpecNormalization:
    """VLM 约束输出归一化（混入绘图元素丢弃、同义类型映射）。"""

    def test_normalize_drops_drawing_elements(self):
        """混入 constraints 的绘图元素应被丢弃（归一化层）。"""
        cons, dropped = normalize_constraints([
            {"type": "ray", "pts": ["A", "B"]},
            {"type": "horizontal", "points": ["A", "B"]},
        ])
        assert len(cons) == 1
        assert cons[0]["type"] == "horizontal"
        assert any("ray" in d for d in dropped)

    def test_normalize_maps_synonyms(self):
        """point_between / perpendicular 等同义类型应被映射（不进入 dropped）。"""
        cons, dropped = normalize_constraints([
            {"type": "point_between", "point": "O", "a": "A", "b": "B"},
            {"type": "perpendicular", "vertex": "A", "a": "B", "b": "C"},
            {"type": "ray", "pts": ["A", "B"]},
        ])
        types = [c["type"] for c in cons]
        assert types == ["between", "angle"]
        # 只有绘图元素（ray）会被记录到 dropped。
        assert any("ray" in d for d in dropped)
        assert not any("perpendicular" in d for d in dropped)
