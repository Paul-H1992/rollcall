#!/usr/bin/env python3
"""
智慧课堂点名提问系统 - 单文件版本
零依赖: 仅需 pip install flask (Python3内置sqlite3)
"""

import sqlite3
import random
import uuid
import json
import threading
import time
from datetime import datetime, timedelta
from pathlib import Path
from flask import Flask, request, jsonify, render_template, send_from_directory
from werkzeug.serving import make_server

app = Flask(__name__)

# ==================== 配置 ====================
DB_PATH = "./data/rollcall.db"
DATA_DIR = Path("./data")
DATA_DIR.mkdir(exist_ok=True)

# ==================== 全局状态（用于实时推送） ====================
# 当前进行中的点名
current_call = {
    'record_id': None,
    'record': None,
    'answerers': [],
    'askers_map': {},
    'started_at': None,
    'active': False
}

# 通知队列（学生端轮询获取）
notification_queue = []
notification_lock = threading.Lock()

def push_notification(notif_type, payload):
    """推送通知给学生端"""
    global notification_queue
    with notification_lock:
        notification_queue.append({
            'type': notif_type,
            'payload': payload,
            'timestamp': datetime.now().isoformat()
        })
        # 只保留最近50条
        if len(notification_queue) > 50:
            notification_queue = notification_queue[-50:]

# ==================== 数据库 ====================
def get_db():
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    return conn

def init_db():
    """初始化数据库表"""
    conn = get_db()
    cursor = conn.cursor()
    cursor.executescript('''
        CREATE TABLE IF NOT EXISTS students (
            id TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            student_no TEXT,
            status TEXT DEFAULT 'normal',
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
        );
        
        CREATE TABLE IF NOT EXISTS call_records (
            id TEXT PRIMARY KEY,
            mode TEXT NOT NULL,
            answerer_cnt INTEGER NOT NULL,
            asker_cnt INTEGER NOT NULL,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        );
        
        CREATE TABLE IF NOT EXISTS call_details (
            id TEXT PRIMARY KEY,
            call_record_id TEXT NOT NULL,
            student_id TEXT NOT NULL,
            role TEXT NOT NULL,
            sort_order INTEGER NOT NULL,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (call_record_id) REFERENCES call_records(id)
        );
        
        CREATE TABLE IF NOT EXISTS cooldowns (
            id TEXT PRIMARY KEY,
            student_id TEXT NOT NULL,
            start_date TEXT NOT NULL,
            end_date TEXT NOT NULL,
            days INTEGER NOT NULL,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        );
        
        CREATE TABLE IF NOT EXISTS questions (
            id TEXT PRIMARY KEY,
            content TEXT NOT NULL,
            answer TEXT,
            criteria TEXT,
            time_limit INTEGER DEFAULT 60,
            stage INTEGER,
            tags TEXT,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        );
        
        CREATE TABLE IF NOT EXISTS tasks (
            id TEXT PRIMARY KEY,
            content TEXT NOT NULL,
            deadline TEXT,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        );
        
        CREATE TABLE IF NOT EXISTS task_feedbacks (
            id TEXT PRIMARY KEY,
            task_id TEXT NOT NULL,
            student_id TEXT NOT NULL,
            feedback_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (task_id) REFERENCES tasks(id),
            UNIQUE(task_id, student_id)
        );
        
        CREATE TABLE IF NOT EXISTS settings (
            key TEXT PRIMARY KEY,
            value TEXT
        );
    ''')
    conn.commit()
    conn.close()

# ==================== 学生管理 ====================

@app.route('/api/students', methods=['GET'])
def get_students():
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('SELECT * FROM students ORDER BY name')
    students = [dict(row) for row in cursor.fetchall()]
    conn.close()
    return jsonify(students)

@app.route('/api/students', methods=['POST'])
def create_student():
    data = request.json
    student_id = str(uuid.uuid4())
    now = datetime.now().isoformat()
    
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute(
        'INSERT INTO students (id, name, student_no, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)',
        (student_id, data['name'], data.get('student_no', ''), 'normal', now, now)
    )
    conn.commit()
    conn.close()
    
    return jsonify({'id': student_id, 'name': data['name'], 'student_no': data.get('student_no', ''), 'status': 'normal'}), 201

@app.route('/api/students/<student_id>', methods=['PUT'])
def update_student(student_id):
    data = request.json
    now = datetime.now().isoformat()
    
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute(
        'UPDATE students SET name=?, student_no=?, status=?, updated_at=? WHERE id=?',
        (data['name'], data.get('student_no', ''), data.get('status', 'normal'), now, student_id)
    )
    conn.commit()
    conn.close()
    
    return jsonify({'message': 'updated'})

@app.route('/api/students/<student_id>', methods=['DELETE'])
def delete_student(student_id):
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('DELETE FROM students WHERE id=?', (student_id,))
    conn.commit()
    conn.close()
    return jsonify({'message': 'deleted'})

@app.route('/api/students/import', methods=['POST'])
def import_students():
    data = request.json
    students = data.get('students', [])
    now = datetime.now().isoformat()
    
    conn = get_db()
    cursor = conn.cursor()
    for s in students:
        student_id = str(uuid.uuid4())
        cursor.execute(
            'INSERT INTO students (id, name, student_no, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)',
            (student_id, s['name'], s.get('student_no', ''), 'normal', now, now)
        )
    conn.commit()
    conn.close()
    
    return jsonify({'message': 'imported', 'count': len(students)}), 201

@app.route('/api/students/<student_id>/status', methods=['PUT'])
def set_student_status(student_id):
    status = request.args.get('status', 'normal')
    if status not in ['normal', 'leave', 'exclude']:
        return jsonify({'error': 'invalid status'}), 400
    
    now = datetime.now().isoformat()
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('UPDATE students SET status=?, updated_at=? WHERE id=?', (status, now, student_id))
    conn.commit()
    conn.close()
    
    return jsonify({'message': 'status updated'})

# ==================== 冷却管理 ====================

@app.route('/api/cooldowns', methods=['GET'])
def get_cooldowns():
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('SELECT * FROM cooldowns ORDER BY created_at DESC')
    cooldowns = [dict(row) for row in cursor.fetchall()]
    conn.close()
    return jsonify(cooldowns)

@app.route('/api/cooldowns/<student_id>', methods=['DELETE'])
def remove_cooldown(student_id):
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('DELETE FROM cooldowns WHERE student_id=?', (student_id,))
    conn.commit()
    conn.close()
    return jsonify({'message': 'cooldown removed'})

@app.route('/api/cooldowns', methods=['POST'])
def add_cooldown_manual():
    """手动设置学生冷却"""
    data = request.json
    student_id = data.get('student_id')
    days = data.get('days', 3)
    
    if not student_id:
        return jsonify({'error': 'student_id required'}), 400
    
    today = datetime.now()
    end_date = (today + timedelta(days=days)).strftime('%Y-%m-%d')
    now = today.isoformat()
    
    conn = get_db()
    cursor = conn.cursor()
    
    # 检查是否已有冷却
    cursor.execute('SELECT id FROM cooldowns WHERE student_id=?', (student_id,))
    existing = cursor.fetchone()
    
    if existing:
        cursor.execute(
            'UPDATE cooldowns SET start_date=?, end_date=?, days=? WHERE student_id=?',
            (today.strftime('%Y-%m-%d'), end_date, days, student_id)
        )
    else:
        cursor.execute(
            'INSERT INTO cooldowns (id, student_id, start_date, end_date, days, created_at) VALUES (?, ?, ?, ?, ?, ?)',
            (str(uuid.uuid4()), student_id, today.strftime('%Y-%m-%d'), end_date, days, now)
        )
    
    conn.commit()
    conn.close()
    
    return jsonify({'message': 'cooldown added', 'days': days})

# ==================== 点名 ====================

@app.route('/api/call/start', methods=['POST'])
def start_call():
    data = request.json
    mode = data.get('mode')  # multi_one / multi_multi
    answerer_cnt = data.get('answerer_cnt', 3)
    asker_cnt = data.get('asker_cnt', 5)
    
    if mode not in ['multi_one', 'multi_multi']:
        return jsonify({'error': 'invalid mode'}), 400
    
    today = datetime.now().strftime('%Y-%m-%d')
    
    conn = get_db()
    cursor = conn.cursor()
    
    # 获取可用学生（排除请假、排除、冷却中）
    cursor.execute('''
        SELECT s.* FROM students s
        WHERE s.status = 'normal'
        AND s.id NOT IN (
            SELECT student_id FROM cooldowns WHERE ? BETWEEN start_date AND end_date
        )
    ''', (today,))
    
    available = [dict(row) for row in cursor.fetchall()]
    
    if len(available) < answerer_cnt:
        conn.close()
        return jsonify({'error': 'not enough available students'}), 400
    
    # 随机抽取回答者
    selected_answerers = random.sample(available, answerer_cnt)
    
    # 创建点名记录
    record_id = str(uuid.uuid4())
    now = datetime.now().isoformat()
    cursor.execute(
        'INSERT INTO call_records (id, mode, answerer_cnt, asker_cnt, created_at) VALUES (?, ?, ?, ?, ?)',
        (record_id, mode, answerer_cnt, asker_cnt, now)
    )
    
    # 保存回答者
    answerers = []
    for i, a in enumerate(selected_answerers):
        cursor.execute(
            'INSERT INTO call_details (id, call_record_id, student_id, role, sort_order, created_at) VALUES (?, ?, ?, ?, ?, ?)',
            (str(uuid.uuid4()), record_id, a['id'], 'answerer', i, now)
        )
        answerers.append(a)
    
    # 多对一模式：抽取提问者
    askers_map = {}
    if mode == 'multi_multi':
        # 获取所有在场学生（包含冷却中的，但排除请假）
        cursor.execute("SELECT * FROM students WHERE status != 'leave'")
        all_students = [dict(row) for row in cursor.fetchall()]
        
        askers = []  # 存储所有提问者详情
        for i, answerer in enumerate(selected_answerers):
            # 排除当前回答者
            pool = [s for s in all_students if s['id'] != answerer['id']]
            if len(pool) < asker_cnt:
                selected_askers = pool
            else:
                selected_askers = random.sample(pool, asker_cnt)
            
            asker_ids = []
            for j, asker in enumerate(selected_askers):
                cursor.execute(
                    'INSERT INTO call_details (id, call_record_id, student_id, role, sort_order, created_at) VALUES (?, ?, ?, ?, ?, ?)',
                    (str(uuid.uuid4()), record_id, asker['id'], 'asker', i * asker_cnt + j, now)
                )
                asker_ids.append(asker['id'])
                askers.append(asker)
            askers_map[str(i)] = asker_ids
    
    conn.commit()
    conn.close()
    
    # 更新全局状态
    current_call['record_id'] = record_id
    current_call['record'] = {
        'id': record_id,
        'mode': mode,
        'answerer_cnt': answerer_cnt,
        'asker_cnt': asker_cnt,
        'created_at': now
    }
    current_call['answerers'] = answerers
    current_call['askers_map'] = askers_map
    current_call['askers'] = askers if mode == 'multi_multi' else []
    current_call['started_at'] = now
    current_call['active'] = True
    
    # 推送通知
    push_notification('call_started', {
        'record_id': record_id,
        'mode': mode,
        'answerers': answerers,
        'askers_map': askers_map,
        'askers': current_call['askers']
    })
    
    return jsonify({
        'record_id': record_id,
        'mode': mode,
        'answerers': answerers,
        'askers_map': askers_map
    }), 201

@app.route('/api/call/state', methods=['GET'])
def get_call_state():
    """获取当前进行中的点名状态（学生端轮询）"""
    if current_call['active'] and current_call['record']:
        return jsonify({
            'active': True,
            'record': current_call['record'],
            'answerers': current_call['answerers'],
            'askers_map': current_call['askers_map'],
            'askers': current_call.get('askers', [])
        })
    return jsonify({'active': False, 'record': None, 'answerers': [], 'askers_map': {}, 'askers': []})

@app.route('/api/call/end', methods=['POST'])
def end_call():
    """结束当前点名"""
    global current_call
    current_call['active'] = False
    current_call['record_id'] = None
    current_call['record'] = None
    current_call['answerers'] = []
    current_call['askers_map'] = {}
    current_call['askers'] = []
    current_call['started_at'] = None
    
    push_notification('call_ended', {})
    return jsonify({'message': 'call ended'})

@app.route('/api/notifications', methods=['GET'])
def get_notifications():
    """获取通知队列（学生端轮询）"""
    with notification_lock:
        # 返回所有未读通知
        notifications = notification_queue.copy()
    return jsonify({'notifications': notifications})

@app.route('/api/notifications/clear', methods=['POST'])
def clear_notifications():
    """清空通知队列"""
    with notification_lock:
        notification_queue.clear()
    return jsonify({'message': 'cleared'})

@app.route('/api/call/records', methods=['GET'])
def get_call_records():
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('SELECT * FROM call_records ORDER BY created_at DESC LIMIT 50')
    records = [dict(row) for row in cursor.fetchall()]
    conn.close()
    return jsonify(records)

@app.route('/api/call/records/<record_id>', methods=['GET'])
def get_call_record_detail(record_id):
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('SELECT * FROM call_records WHERE id=?', (record_id,))
    record = dict(cursor.fetchone()) if cursor.fetchone else None
    
    if not record:
        conn.close()
        return jsonify({'error': 'record not found'}), 404
    
    cursor.execute('''
        SELECT cd.*, s.name, s.student_no FROM call_details cd
        LEFT JOIN students s ON cd.student_id = s.id
        WHERE cd.call_record_id = ?
        ORDER BY cd.sort_order
    ''', (record_id,))
    
    details = [dict(row) for row in cursor.fetchall()]
    conn.close()
    
    return jsonify({'record': record, 'details': details})

@app.route('/api/call/cooldown', methods=['POST'])
def add_cooldown():
    """点名结束后，为回答者添加冷却期"""
    data = request.json
    answerer_ids = data.get('answerer_ids', [])
    days = data.get('days', 3)  # 默认3天冷却
    
    today = datetime.now()
    end_date = (today + timedelta(days=days)).strftime('%Y-%m-%d')
    now = today.isoformat()
    
    conn = get_db()
    cursor = conn.cursor()
    
    for student_id in answerer_ids:
        # 检查是否已有冷却
        cursor.execute('SELECT id FROM cooldowns WHERE student_id=?', (student_id,))
        existing = cursor.fetchone()
        
        if existing:
            # 更新现有冷却
            cursor.execute(
                'UPDATE cooldowns SET start_date=?, end_date=?, days=? WHERE student_id=?',
                (today.strftime('%Y-%m-%d'), end_date, days, student_id)
            )
        else:
            cursor.execute(
                'INSERT INTO cooldowns (id, student_id, start_date, end_date, days, created_at) VALUES (?, ?, ?, ?, ?, ?)',
                (str(uuid.uuid4()), student_id, today.strftime('%Y-%m-%d'), end_date, days, now)
            )
    
    conn.commit()
    conn.close()
    
    return jsonify({'message': 'cooldown added', 'days': days})

# ==================== 题目管理 ====================

@app.route('/api/questions', methods=['GET'])
def get_questions():
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('SELECT * FROM questions ORDER BY created_at DESC')
    questions = [dict(row) for row in cursor.fetchall()]
    conn.close()
    return jsonify(questions)

@app.route('/api/questions', methods=['POST'])
def create_question():
    data = request.json
    question_id = str(uuid.uuid4())
    now = datetime.now().isoformat()
    
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute(
        'INSERT INTO questions (id, content, answer, criteria, time_limit, stage, tags, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)',
        (question_id, data['content'], data.get('answer', ''), data.get('criteria', ''), 
         data.get('time_limit', 60), data.get('stage'), data.get('tags', ''), now)
    )
    conn.commit()
    conn.close()
    
    return jsonify({'id': question_id, 'message': 'created'}), 201

@app.route('/api/questions/<question_id>', methods=['DELETE'])
def delete_question(question_id):
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('DELETE FROM questions WHERE id=?', (question_id,))
    conn.commit()
    conn.close()
    return jsonify({'message': 'deleted'})

@app.route('/api/stages', methods=['GET'])
def get_stages():
    """返回16阶段课程体系"""
    stages = {
        1: {"name": "大模型开发入门", "tags": ["Ollama", "Python调用API", "Streamlit"], "level": "基础认知"},
        2: {"name": "Python语言进阶", "tags": ["面向对象", "网络编程", "闭包/装饰器"], "level": "基础认知"},
        3: {"name": "数据处理与统计分析", "tags": ["Pandas", "MySQL", "数据可视化"], "level": "基础认知"},
        4: {"name": "机器学习基础", "tags": ["KNN", "线性回归", "决策树", "集成学习"], "level": "基础认知"},
        5: {"name": "深度学习基础", "tags": ["神经网络", "反向传播", "Pytorch", "CNN/RNN"], "level": "原理剖析"},
        6: {"name": "NLP自然语言处理基础", "tags": ["Transformer", "BERT", "迁移学习"], "level": "原理剖析"},
        7: {"name": "文本分类与模型优化", "tags": ["FastText", "BERT微调", "量化/剪枝"], "level": "原理剖析"},
        8: {"name": "RAG检索增强生成", "tags": ["LangChain", "向量数据库", "RAG系统"], "level": "原理剖析"},
        9: {"name": "Agent智能体开发", "tags": ["Dify", "CrewAI", "智能体机制"], "level": "场景应用"},
        10: {"name": "大模型微调", "tags": ["LoRA", "P-Tuning", "医疗问答"], "level": "场景应用"},
        11: {"name": "企业级大模型平台", "tags": ["阿里PAI", "虚拟试衣", "Diffusion"], "level": "场景应用"},
        12: {"name": "知识图谱与问答系统", "tags": ["Neo4j", "NER", "关系抽取"], "level": "场景应用"},
        13: {"name": "NLP高级实战", "tags": ["BERT+BiLSTM+CRF", "API部署"], "level": "架构与拓展"},
        14: {"name": "模型部署", "tags": ["Flask", "Gradio", "Docker容器化", "模型服务封装"], "level": "架构与拓展"},
        15: {"name": "图像分析与计算机视觉", "tags": ["ResNet", "Unet", "多模态基础"], "level": "架构与拓展"},
        16: {"name": "多模态大模型（AIGC）", "tags": ["Stable Diffusion", "CLIP", "图像生成"], "level": "架构与拓展"},
    }
    return jsonify(stages)

@app.route('/api/questions/generate', methods=['POST'])
def generate_question():
    """AI生成面试题（需配置API Key）"""
    data = request.json
    stage = data.get('stage', 1)
    tags = data.get('tags', '')
    
    # 16阶段课程体系
    stages = {
        1: {"name": "大模型开发入门", "level": "基础认知"},
        2: {"name": "Python语言进阶", "level": "基础认知"},
        3: {"name": "数据处理与统计分析", "level": "基础认知"},
        4: {"name": "机器学习基础", "level": "基础认知"},
        5: {"name": "深度学习基础", "level": "原理剖析"},
        6: {"name": "NLP自然语言处理基础", "level": "原理剖析"},
        7: {"name": "文本分类与模型优化", "level": "原理剖析"},
        8: {"name": "RAG检索增强生成", "level": "原理剖析"},
        9: {"name": "Agent智能体开发", "level": "场景应用"},
        10: {"name": "大模型微调", "level": "场景应用"},
        11: {"name": "企业级大模型平台", "level": "场景应用"},
        12: {"name": "知识图谱与问答系统", "level": "场景应用"},
        13: {"name": "NLP高级实战", "level": "架构与拓展"},
        14: {"name": "模型部署", "level": "架构与拓展"},
        15: {"name": "图像分析与计算机视觉", "level": "架构与拓展"},
        16: {"name": "多模态大模型（AIGC）", "level": "架构与拓展"},
    }
    
    stage_info = stages.get(stage, stages[1])
    
    # TODO: 接入MiniMax API时替换此处
    # 目前返回示例题目，实际使用时调用AI API
    question = {
        "content": f"请简述{stage_info['name']}的核心原理，并说明在实际项目中的应用场景。",
        "answer": f"【参考答案要点】\n1. 核心原理（30%）\n2. 关键步骤（30%）\n3. 实际应用场景（40%）",
        "criteria": f"1. 理解正确 - 30%\n2. 表达清晰 - 30%\n3. 有实践经验 - 40%",
        "time_limit": 120,
        "stage": stage,
        "tags": tags,
        "is_ai_generated": True
    }
    
    return jsonify(question)

# ==================== SSE实时推送 ====================

# 存储SSE客户端
sse_clients = []
sse_lock = threading.Lock()

@app.route('/api/stream')
def sse_stream():
    """Server-Sent Events 实时推送（学生端使用）"""
    from flask import Response as FlaskResponse
    
    def generate():
        while True:
            time.sleep(3)  # 每3秒发送心跳
            yield 'data: ping\n\n'
    
    return FlaskResponse(generate(), mimetype='text/event-stream')

@app.route('/api/broadcast', methods=['POST'])
def broadcast_message():
    """广播消息给所有SSE客户端"""
    data = request.json
    msg_type = data.get('type', 'update')
    payload = data.get('payload', {})
    
    message = f"data: {json.dumps({'type': msg_type, 'payload': payload})}\n\n"
    
    # 这里简单记录，实际生产环境需要存储并让客户端重新获取
    return jsonify({'message': 'broadcast sent'})

# ==================== 任务管理 ====================

@app.route('/api/tasks', methods=['GET'])
def get_tasks():
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('SELECT * FROM tasks ORDER BY created_at DESC LIMIT 20')
    tasks = [dict(row) for row in cursor.fetchall()]
    conn.close()
    return jsonify(tasks)

@app.route('/api/tasks', methods=['POST'])
def create_task():
    data = request.json
    task_id = str(uuid.uuid4())
    now = datetime.now().isoformat()
    
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute(
        'INSERT INTO tasks (id, content, deadline, created_at) VALUES (?, ?, ?, ?)',
        (task_id, data['content'], data.get('deadline', ''), now)
    )
    conn.commit()
    conn.close()
    
    # 广播任务给所有学生端
    broadcast_to_students({'type': 'new_task', 'task': {'id': task_id, 'content': data['content'], 'deadline': data.get('deadline', '')}})
    
    return jsonify({'id': task_id, 'message': 'created'}), 201

@app.route('/api/tasks/<task_id>', methods=['DELETE'])
def delete_task(task_id):
    """删除任务"""
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('DELETE FROM task_feedbacks WHERE task_id=?', (task_id,))
    cursor.execute('DELETE FROM tasks WHERE id=?', (task_id,))
    conn.commit()
    conn.close()
    return jsonify({'message': 'deleted'})

@app.route('/api/tasks/<task_id>/feedback', methods=['POST'])
def task_feedback(task_id):
    # 支持所有格式：JSON / form-data / URL参数
    try:
        data = request.json or {}
    except:
        data = {}
    student_identifier = data.get('student_id') or request.values.get('student_id')
    
    if not student_identifier:
        return jsonify({'error': 'student_id required', 'received': request.values.to_dict()}), 400
    
    conn = get_db()
    cursor = conn.cursor()
    
    # 根据姓名/学号查找学生DB ID
    cursor.execute('SELECT id, name, student_no FROM students WHERE name=? OR student_no=?', (student_identifier, student_identifier))
    student = cursor.fetchone()
    
    if not student:
        conn.close()
        return jsonify({'error': 'student not found', 'identifier': student_identifier}), 404
    
    student_db_id = student['id']
    student_name = student['name']
    student_no = student['student_no']
    
    # 检查是否已反馈（用真实DB ID）
    cursor.execute('SELECT id FROM task_feedbacks WHERE task_id=? AND student_id=?', (task_id, student_db_id))
    if cursor.fetchone():
        conn.close()
        return jsonify({'message': 'already feedback'}), 200
    
    # 记录反馈（用真实DB ID）
    feedback_id = str(uuid.uuid4())
    now = datetime.now().isoformat()
    cursor.execute(
        'INSERT INTO task_feedbacks (id, task_id, student_id, feedback_at) VALUES (?, ?, ?, ?)',
        (feedback_id, task_id, student_db_id, now)
    )
    conn.commit()
    
    # 获取反馈统计（现在join能正常工作了）
    cursor.execute('''
        SELECT tf.*, s.name, s.student_no FROM task_feedbacks tf
        LEFT JOIN students s ON tf.student_id = s.id
        WHERE tf.task_id = ?
        ORDER BY tf.feedback_at
    ''', (task_id,))
    feedbacks = [dict(row) for row in cursor.fetchall()]
    
    cursor.execute('SELECT COUNT(*) FROM students WHERE status != "exclude"')
    total = cursor.fetchone()[0]
    
    conn.close()
    
    # 广播反馈更新（包含学生信息）
    broadcast_to_students({
        'type': 'feedback_update', 
        'task_id': task_id, 
        'feedbacks': feedbacks, 
        'total': total,
        'student_name': student_name  # 新增：方便前端显示
    })
    
    return jsonify({
        'message': 'feedback recorded', 
        'feedbacks': feedbacks, 
        'total': total,
        'student_name': student_name
    })

@app.route('/api/tasks/<task_id>/feedbacks', methods=['GET'])
def get_task_feedbacks(task_id):
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('''
        SELECT tf.*, s.name, s.student_no FROM task_feedbacks tf
        LEFT JOIN students s ON tf.student_id = s.id
        WHERE tf.task_id = ?
        ORDER BY tf.feedback_at
    ''', (task_id,))
    feedbacks = [dict(row) for row in cursor.fetchall()]
    
    cursor.execute('SELECT COUNT(*) FROM students WHERE status != "exclude"')
    total = cursor.fetchone()[0]
    
    conn.close()
    
    return jsonify({'feedbacks': feedbacks, 'total': total})

@app.route('/api/tasks/<task_id>/stats', methods=['GET'])
def get_task_stats(task_id):
    """获取任务反馈统计（供老师端展示已确认名单和百分比）"""
    conn = get_db()
    cursor = conn.cursor()
    
    # 获取已反馈的学生
    cursor.execute('''
        SELECT tf.*, s.name, s.student_no FROM task_feedbacks tf
        LEFT JOIN students s ON tf.student_id = s.id
        WHERE tf.task_id = ?
        ORDER BY tf.feedback_at
    ''', (task_id,))
    feedbacks = [dict(row) for row in cursor.fetchall()]
    
    # 获取总学生数（排除exclude）
    cursor.execute('SELECT COUNT(*) FROM students WHERE status != "exclude"')
    total = cursor.fetchone()[0]
    
    # 获取任务信息
    cursor.execute('SELECT * FROM tasks WHERE id=?', (task_id,))
    task_row = cursor.fetchone()
    task = dict(task_row) if task_row else None
    
    conn.close()
    
    confirmed_count = len(feedbacks)
    percentage = round(confirmed_count / total * 100, 1) if total > 0 else 0
    
    return jsonify({
        'task': task,
        'confirmed_count': confirmed_count,
        'total': total,
        'percentage': percentage,
        'confirmed_students': [{'id': f['student_id'], 'name': f['name'], 'student_no': f['student_no'], 'feedback_at': f['feedback_at']} for f in feedbacks]
    })

# ==================== 设置 ====================

@app.route('/api/settings/cooldown_days', methods=['GET'])
def get_cooldown_days():
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('SELECT value FROM settings WHERE key="cooldown_days"')
    row = cursor.fetchone()
    conn.close()
    return jsonify({'days': int(row[0]) if row else 3})

@app.route('/api/settings/cooldown_days', methods=['PUT'])
def set_cooldown_days():
    data = request.json
    days = data.get('days', 3)
    
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('INSERT OR REPLACE INTO settings (key, value) VALUES ("cooldown_days", ?)', (str(days),))
    conn.commit()
    conn.close()
    
    return jsonify({'message': 'updated', 'days': days})

# ==================== 导出 ====================

@app.route('/api/export/records', methods=['GET'])
def export_records_json():
    conn = get_db()
    cursor = conn.cursor()
    
    cursor.execute('SELECT * FROM call_records ORDER BY created_at DESC')
    records = [dict(row) for row in cursor.fetchall()]
    
    for record in records:
        cursor.execute('''
            SELECT s.name, s.student_no, cd.role FROM call_details cd
            LEFT JOIN students s ON cd.student_id = s.id
            WHERE cd.call_record_id = ?
            ORDER BY cd.sort_order
        ''', (record['id'],))
        
        details = [dict(row) for row in cursor.fetchall()]
        record['details'] = details
    
    conn.close()
    return jsonify(records)

@app.route('/api/export/records/csv')
def export_records_csv():
    """导出点名记录为CSV格式（Excel可打开）"""
    import csv
    import io
    
    conn = get_db()
    cursor = conn.cursor()
    
    cursor.execute('SELECT * FROM call_records ORDER BY created_at DESC')
    records = [dict(row) for row in cursor.fetchall()]
    
    output = io.StringIO()
    writer = csv.writer(output)
    writer.writerow(['时间', '模式', '回答者人数', '提问者人数', '回答者名单', '提问者名单'])
    
    for record in records:
        cursor.execute('''
            SELECT s.name, cd.role FROM call_details cd
            LEFT JOIN students s ON cd.student_id = s.id
            WHERE cd.call_record_id = ?
            ORDER BY cd.sort_order
        ''', (record['id'],))
        
        details = [dict(row) for row in cursor.fetchall()]
        answerers = [d['name'] for d in details if d['role'] == 'answerer']
        askers = [d['name'] for d in details if d['role'] == 'asker']
        
        writer.writerow([
            record['created_at'],
            '多对一' if record['mode'] == 'multi_multi' else '多人问答',
            record['answerer_cnt'],
            record['asker_cnt'],
            '; '.join(answerers),
            '; '.join(askers)
        ])
    
    conn.close()
    
    output.seek(0)
    return output.getvalue(), 200, {
        'Content-Type': 'text/csv; charset=utf-8',
        'Content-Disposition': 'attachment; filename=rollcall_records.csv'
    }

# ==================== 统计 ====================

@app.route('/api/stats', methods=['GET'])
def get_stats():
    conn = get_db()
    cursor = conn.cursor()
    
    cursor.execute('SELECT COUNT(*) FROM students WHERE status != "exclude"')
    total_students = cursor.fetchone()[0]
    
    cursor.execute('SELECT COUNT(*) FROM cooldowns')
    active_cooldowns = cursor.fetchone()[0]
    
    cursor.execute('SELECT COUNT(*) FROM questions')
    total_questions = cursor.fetchone()[0]
    
    cursor.execute('SELECT COUNT(*) FROM call_records')
    total_calls = cursor.fetchone()[0]
    
    conn.close()
    
    return jsonify({
        'total_students': total_students,
        'active_cooldowns': active_cooldowns,
        'total_questions': total_questions,
        'total_calls': total_calls
    })

# ==================== WebSocket模拟（SSE） ====================

# 存储学生端连接（用于推送）
student_connections = []
student_lock = threading.Lock()

def broadcast_to_students(message):
    """向所有学生端推送消息"""
    msg_type = message.get('type', 'update')
    payload = message.get('payload', {})
    push_notification(msg_type, payload)

# ==================== 前端页面 ====================

@app.route('/')
def index():
    return render_template('teacher.html')

@app.route('/teacher')
def teacher_panel():
    return render_template('teacher.html')

@app.route('/student')
def student_panel():
    return render_template('student.html')

# ==================== 启动 ====================

def run_server(port=8080):
    """启动服务器"""
    print(f"""
╔═══════════════════════════════════════════════════════╗
║       智慧课堂点名提问系统 v1.0                       ║
╠═══════════════════════════════════════════════════════╣
║  教师端: http://localhost:{port}/teacher              ║
║  学生端: http://localhost:{port}/student               ║
║                                                       ║
║  API文档: http://localhost:{port}/api                  ║
╚═══════════════════════════════════════════════════════╝
    """)
    app.run(host='0.0.0.0', port=port, debug=False, threaded=True)

if __name__ == '__main__':
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument('--port', type=int, default=8080)
    args = parser.parse_args()
    init_db()
    run_server(args.port)
