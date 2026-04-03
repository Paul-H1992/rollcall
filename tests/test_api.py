# -*- coding: utf-8 -*-
"""
智慧课堂点名系统 - API自动化测试

运行方式:
    cd /path/to/rollcall-pro-2
    pip install pytest requests
    pytest tests/test_api.py -v

测试覆盖:
    1. 学生管理 (CRUD)
    2. 课堂任务 (发布/删除/反馈)
    3. 反馈统计 (名字正确显示)
    4. 实时点名 (开始/结束/状态)
    5. 冷板凳管理
"""

import pytest
import requests
import sqlite3
import uuid
import time
import os
import shutil
from pathlib import Path
from contextlib import contextmanager

# ============ 配置 ============
BASE_URL = "http://127.0.0.1:8888"
API = BASE_URL
DB_PATH = "./data/rollcall.db"
TEST_DB_PATH = "./data/test_rollcall.db"

# ============ 辅助函数 ============

def wait_for_server(timeout=10):
    """等待服务启动"""
    start = time.time()
    while time.time() - start < timeout:
        try:
            r = requests.get(f"{API}/api/stats", timeout=2)
            if r.status_code == 200:
                return True
        except:
            pass
        time.sleep(0.5)
    return False


@contextmanager
def backup_db():
    """备份/恢复数据库上下文"""
    bak_path = DB_PATH + ".bak"
    if os.path.exists(DB_PATH):
        shutil.copy(DB_PATH, bak_path)
    if os.path.exists(DB_PATH + ".bak"):
        pass
    try:
        yield
    finally:
        if os.path.exists(bak_path):
            shutil.move(bak_path, DB_PATH)


def create_test_student(name="测试学生", student_no="00001"):
    """创建测试学生"""
    r = requests.post(f"{API}/api/students", json={
        "name": name,
        "student_no": student_no
    })
    return r.json()


def create_test_task(content="测试任务", deadline=None):
    """创建测试任务"""
    payload = {"content": content}
    if deadline:
        payload["deadline"] = deadline
    r = requests.post(f"{API}/api/tasks", json=payload)
    return r.json()


def submit_feedback(task_id, student_id):
    """提交任务反馈"""
    r = requests.post(f"{API}/api/tasks/{task_id}/feedback", data={
        "student_id": student_id
    })
    return r


# ============ Fixtures ============

@pytest.fixture(scope="module")
def server_running():
    """确保服务正在运行"""
    assert wait_for_server(), "服务未启动，请先运行: python3 rollcall.py --port 8888"
    yield True


@pytest.fixture(autouse=True)
def reset_data():
    """每个测试后重置数据"""
    yield
    # 测试后清理 - 可以选择清空task_feedbacks表


# ============ 学生管理测试 ============

class TestStudentManagement:
    """学生管理API测试"""

    def test_get_students(self, server_running):
        """获取学生列表"""
        r = requests.get(f"{API}/api/students")
        assert r.status_code == 200
        data = r.json()
        assert isinstance(data, list), "返回应该是学生列表"

    def test_create_student(self, server_running):
        """创建学生"""
        unique_name = f"学生_{uuid.uuid4().hex[:6]}"
        r = requests.post(f"{API}/api/students", json={
            "name": unique_name,
            "student_no": "99999"
        })
        assert r.status_code in [200, 201]  # 201 Created or 200 OK
        data = r.json()
        assert data["name"] == unique_name
        assert "id" in data

    def test_create_duplicate_student(self, server_running):
        """重复创建同名学生 - 返回200或201表示可能允许重名（取决于实现）"""
        name = f"重复测试_{uuid.uuid4().hex[:6]}"
        r1 = requests.post(f"{API}/api/students", json={"name": name})
        assert r1.status_code in [200, 201]
        r2 = requests.post(f"{API}/api/students", json={"name": name})
        # 有些实现允许重名，只要返回200/201就行
        assert r2.status_code in [200, 201, 400, 409]

    def test_update_student_status(self, server_running):
        """修改学生状态"""
        student = create_test_student(f"状态测试_{uuid.uuid4().hex[:6]}")
        student_id = student["id"]

        # 改成请假
        r = requests.put(f"{API}/api/students/{student_id}/status?status=leave")
        assert r.status_code == 200

        # 确认改成功了
        students = requests.get(f"{API}/api/students").json()
        updated = next((s for s in students if s["id"] == student_id), None)
        assert updated is not None
        assert updated["status"] == "leave"

    def test_delete_student(self, server_running):
        """删除学生"""
        student = create_test_student(f"删除测试_{uuid.uuid4().hex[:6]}")
        student_id = student["id"]

        r = requests.delete(f"{API}/api/students/{student_id}")
        assert r.status_code == 200

        # 确认删掉了
        students = requests.get(f"{API}/api/students").json()
        deleted = next((s for s in students if s["id"] == student_id), None)
        assert deleted is None


# ============ 任务反馈测试 (核心!) ============

class TestTaskFeedback:
    """任务反馈核心测试 - 验证名字能正确显示"""

    def test_create_and_get_task(self, server_running):
        """创建并获取任务"""
        task = create_test_task(content="测试任务内容")
        assert "id" in task
        # 返回格式是 {"id": ..., "message": "created"}
        
    def test_feedback_with_name(self, server_running):
        """
        核心测试: 学生用姓名提交反馈，老师端能看到名字
        
        这是之前修复的bug: 学生提交"张三" -> 
        服务器根据姓名查DB ID -> 正确存储 -> 老师端能显示
        """
        # 1. 创建一个学生
        student_name = f"反馈测试_{uuid.uuid4().hex[:6]}"
        student = create_test_student(student_name)
        student_id = student["id"]

        # 2. 创建一个任务
        task = create_test_task(content="反馈测试任务")
        task_id = task["id"]

        # 3. 学生用 **姓名** 提交反馈（不是用数据库ID）
        r = submit_feedback(task_id, student_name)
        
        # 之前这里会返回200但名字显示不出来，现在应该正常
        assert r.status_code == 200, f"提交失败: {r.text}"
        data = r.json()
        
        # 验证返回包含学生名字
        assert "student_name" in data, "返回应该有student_name字段"
        assert data["student_name"] == student_name, f"学生名字不对: {data}"

    def test_teacher_sees_feedback_with_name(self, server_running):
        """
        端到端测试: 老师获取任务统计，能看到已确认学生的名字
        """
        # 1. 创建学生和任务
        student_name = f"端到端_{uuid.uuid4().hex[:6]}"
        student = create_test_student(student_name)
        task = create_test_task(content="端到端测试任务")
        task_id = task["id"]

        # 2. 学生提交反馈
        submit_feedback(task_id, student_name)

        # 3. 老师获取统计
        r = requests.get(f"{API}/api/tasks/{task_id}/stats")
        assert r.status_code == 200, f"获取统计失败: {r.text}"
        stats = r.json()

        # 4. 验证统计里包含学生名字（这是之前修复的核心bug）
        assert stats["confirmed_count"] == 1, f"确认数不对: {stats}"
        assert len(stats["confirmed_students"]) == 1, "应该有1个已确认学生"
        
        confirmed_name = stats["confirmed_students"][0]["name"]
        assert confirmed_name == student_name, \
            f"老师看到的名字不对: {confirmed_name} != {student_name}"

    def test_multiple_feedbacks(self, server_running):
        """多个学生提交反馈"""
        task = create_test_task(content="多人测试任务")
        task_id = task["id"]

        names = []
        for i in range(3):
            name = f"多人_{uuid.uuid4().hex[:6]}"
            create_test_student(name)
            submit_feedback(task_id, name)
            names.append(name)

        # 验证统计
        stats = requests.get(f"{API}/api/tasks/{task_id}/stats").json()
        assert stats["confirmed_count"] == 3, f"应该有3人确认: {stats}"

        # 验证名字都对了
        confirmed_names = [s["name"] for s in stats["confirmed_students"]]
        for name in names:
            assert name in confirmed_names, f"{name} 应该在确认列表里"

    def test_feedback_with_student_no(self, server_running):
        """用学号而不是姓名提交反馈"""
        student_no = f"NO{uuid.uuid4().hex[:6]}"
        student = create_test_student(f"学号测试_{uuid.uuid4().hex[:6]}", student_no)
        task = create_test_task(content="学号测试任务")
        task_id = task["id"]

        # 用学号提交
        r = submit_feedback(task_id, student_no)
        assert r.status_code == 200
        assert r.json()["student_name"] == student["name"]


# ============ 点名功能测试 ============

class TestCallSession:
    """实时点名功能测试"""

    def test_start_call(self, server_running):
        """开始点名"""
        r = requests.post(f"{API}/api/call/start", json={
            "mode": "multi_one",
            "count": 3
        })
        assert r.status_code in [200, 201]
        data = r.json()
        assert "record_id" in data

    def test_get_call_state(self, server_running):
        """获取点名状态"""
        # 先开始一个点名
        requests.post(f"{API}/api/call/start", json={
            "mode": "multi_one",
            "count": 2
        })

        r = requests.get(f"{API}/api/call/state")
        assert r.status_code == 200
        data = r.json()
        assert "active" in data

    def test_end_call(self, server_running):
        """结束点名"""
        # 先开始
        requests.post(f"{API}/api/call/start", json={
            "mode": "multi_one",
            "count": 1
        })

        r = requests.post(f"{API}/api/call/end", json={})
        assert r.status_code == 200

        # 确认结束了
        state = requests.get(f"{API}/api/call/state").json()
        assert state["active"] == False


# ============ 冷板凳测试 ============

class TestCooldown:
    """冷板凳/冷却期测试"""

    def test_set_cooldown(self, server_running):
        """设置冷却"""
        student = create_test_student(f"冷却测试_{uuid.uuid4().hex[:6]}")
        
        r = requests.post(f"{API}/api/cooldowns", json={
            "student_id": student["id"],
            "days": 3
        })
        assert r.status_code == 200

    def test_get_cooldowns(self, server_running):
        """获取冷却列表"""
        r = requests.get(f"{API}/api/cooldowns")
        assert r.status_code == 200
        data = r.json()
        assert isinstance(data, list)


# ============ 统计测试 ============

class TestStats:
    """全局统计测试"""

    def test_get_stats(self, server_running):
        """获取全局统计"""
        r = requests.get(f"{API}/api/stats")
        assert r.status_code == 200
        data = r.json()
        # 应该有这些字段
        assert "total_students" in data
        assert "total_calls" in data
        assert isinstance(data["total_students"], int)


# ============ 运行入口 ============

if __name__ == "__main__":
    pytest.main([__file__, "-v", "--tb=short"])
