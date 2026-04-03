package db

import (
	"database/sql"
	"fmt"
	"math/rand"
	"rollcall-pro/internal/models"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

func New(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}
	return db, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) migrate() error {
	schema := `
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
		"order" INTEGER NOT NULL,
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
	`
	_, err := d.conn.Exec(schema)
	return err
}

// ---- 学生管理 ----

func (d *DB) CreateStudent(s *models.Student) error {
	s.ID = generateUUID()
	s.CreatedAt = time.Now()
	s.UpdatedAt = time.Now()
	if s.Status == "" {
		s.Status = "normal"
	}
	_, err := d.conn.Exec(
		"INSERT INTO students (id, name, student_no, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		s.ID, s.Name, s.StudentNo, s.Status, s.CreatedAt, s.UpdatedAt,
	)
	return err
}

func (d *DB) GetStudents() ([]models.Student, error) {
	rows, err := d.conn.Query("SELECT id, name, student_no, status, created_at, updated_at FROM students ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var students []models.Student
	for rows.Next() {
		var s models.Student
		rows.Scan(&s.ID, &s.Name, &s.StudentNo, &s.Status, &s.CreatedAt, &s.UpdatedAt)
		students = append(students, s)
	}
	return students, nil
}

func (d *DB) GetStudentByID(id string) (*models.Student, error) {
	var s models.Student
	err := d.conn.QueryRow(
		"SELECT id, name, student_no, status, created_at, updated_at FROM students WHERE id = ?", id,
	).Scan(&s.ID, &s.Name, &s.StudentNo, &s.Status, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (d *DB) UpdateStudent(s *models.Student) error {
	s.UpdatedAt = time.Now()
	_, err := d.conn.Exec(
		"UPDATE students SET name = ?, student_no = ?, status = ?, updated_at = ? WHERE id = ?",
		s.Name, s.StudentNo, s.Status, s.UpdatedAt, s.ID,
	)
	return err
}

func (d *DB) DeleteStudent(id string) error {
	_, err := d.conn.Exec("DELETE FROM students WHERE id = ?", id)
	return err
}

func (d *DB) BatchCreateStudents(students []models.CreateStudentReq) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare("INSERT INTO students (id, name, student_no, status, created_at, updated_at) VALUES (?, ?, ?, 'normal', ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now()
	for _, s := range students {
		id := generateUUID()
		_, err := stmt.Exec(id, s.Name, s.StudentNo, now, now)
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ---- 冷却管理 ----

func (d *DB) IsInCooldown(studentID string) (bool, error) {
	today := time.Now().Format("2006-01-02")
	var count int
	err := d.conn.QueryRow(
		"SELECT COUNT(*) FROM cooldowns WHERE student_id = ? AND ? BETWEEN start_date AND end_date",
		studentID, today,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (d *DB) GetAvailableStudents() ([]models.Student, error) {
	students, err := d.GetStudents()
	if err != nil {
		return nil, err
	}
	var available []models.Student
	for _, s := range students {
		if s.Status != "normal" {
			continue
		}
		inCooldown, _ := d.IsInCooldown(s.ID)
		if !inCooldown {
			available = append(available, s)
		}
	}
	return available, nil
}

func (d *DB) AddCooldown(studentID string, days int) error {
	start := time.Now()
	end := start.AddDate(0, 0, days)
	id := generateUUID()
	_, err := d.conn.Exec(
		"INSERT INTO cooldowns (id, student_id, start_date, end_date, days, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		id, studentID, start.Format("2006-01-02"), end.Format("2006-01-02"), days, time.Now(),
	)
	return err
}

func (d *DB) RemoveCooldown(studentID string) error {
	_, err := d.conn.Exec("DELETE FROM cooldowns WHERE student_id = ?", studentID)
	return err
}

func (d *DB) GetCooldowns() ([]models.Cooldown, error) {
	rows, err := d.conn.Query("SELECT id, student_id, start_date, end_date, days, created_at FROM cooldowns ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cooldowns []models.Cooldown
	for rows.Next() {
		var c models.Cooldown
		rows.Scan(&c.ID, &c.StudentID, &c.StartDate, &c.EndDate, &c.Days, &c.CreatedAt)
		cooldowns = append(cooldowns, c)
	}
	return cooldowns, nil
}

// ---- 点名记录 ----

func (d *DB) CreateCallRecord(record *models.CallRecord) error {
	record.ID = generateUUID()
	record.CreatedAt = time.Now()
	_, err := d.conn.Exec(
		"INSERT INTO call_records (id, mode, answerer_cnt, asker_cnt, created_at) VALUES (?, ?, ?, ?, ?)",
		record.ID, record.Mode, record.AnswererCnt, record.AskerCnt, record.CreatedAt,
	)
	return err
}

func (d *DB) CreateCallDetail(detail *models.CallDetail) error {
	detail.ID = generateUUID()
	detail.CreatedAt = time.Now()
	_, err := d.conn.Exec(
		"INSERT INTO call_details (id, call_record_id, student_id, role, \"order\", created_at) VALUES (?, ?, ?, ?, ?, ?)",
		detail.ID, detail.CallRecordID, detail.StudentID, detail.Role, detail.Order, detail.CreatedAt,
	)
	return err
}

func (d *DB) GetCallRecords() ([]models.CallRecord, error) {
	rows, err := d.conn.Query("SELECT id, mode, answerer_cnt, asker_cnt, created_at FROM call_records ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []models.CallRecord
	for rows.Next() {
		var r models.CallRecord
		rows.Scan(&r.ID, &r.Mode, &r.AnswererCnt, &r.AskerCnt, &r.CreatedAt)
		records = append(records, r)
	}
	return records, nil
}

// ---- 随机抽取 ----

func (d *DB) RandomSelectAnswerers(n int) ([]models.Student, error) {
	available, err := d.GetAvailableStudents()
	if err != nil {
		return nil, err
	}
	if len(available) < n {
		n = len(available)
	}
	// Fisher-Yates shuffle
	rand.Shuffle(len(available), func(i, j int) {
		available[i], available[j] = available[j], available[i]
	})
	return available[:n], nil
}

func (d *DB) RandomSelectAskers(n int, excludeIDs []string) ([]models.Student, error) {
	students, err := d.GetStudents()
	if err != nil {
		return nil, err
	}
	// 排除请假学生
	var available []models.Student
	for _, s := range students {
		if s.Status == "leave" || s.Status == "exclude" {
			continue
		}
		available = append(available, s)
	}
	// 排除指定ID
	if len(excludeIDs) > 0 {
		excludeMap := make(map[string]bool)
		for _, id := range excludeIDs {
			excludeMap[id] = true
		}
		var filtered []models.Student
		for _, s := range available {
			if !excludeMap[s.ID] {
				filtered = append(filtered, s)
			}
		}
		available = filtered
	}
	if len(available) < n {
		n = len(available)
	}
	rand.Shuffle(len(available), func(i, j int) {
		available[i], available[j] = available[j], available[i]
	})
	return available[:n], nil
}

// ---- 题目管理 ----

func (d *DB) CreateQuestion(q *models.Question) error {
	q.ID = generateUUID()
	q.CreatedAt = time.Now()
	_, err := d.conn.Exec(
		"INSERT INTO questions (id, content, answer, criteria, time_limit, stage, tags, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		q.ID, q.Content, q.Answer, q.Criteria, q.TimeLimit, q.Stage, q.Tags, q.CreatedAt,
	)
	return err
}

func (d *DB) GetQuestions() ([]models.Question, error) {
	rows, err := d.conn.Query("SELECT id, content, answer, criteria, time_limit, stage, tags, created_at FROM questions ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var questions []models.Question
	for rows.Next() {
		var q models.Question
		rows.Scan(&q.ID, &q.Content, &q.Answer, &q.Criteria, &q.TimeLimit, &q.Stage, &q.Tags, &q.CreatedAt)
		questions = append(questions, q)
	}
	return questions, nil
}

func (d *DB) DeleteQuestion(id string) error {
	_, err := d.conn.Exec("DELETE FROM questions WHERE id = ?", id)
	return err
}

// ---- 任务管理 ----

func (d *DB) CreateTask(t *models.Task) error {
	t.ID = generateUUID()
	t.CreatedAt = time.Now()
	_, err := d.conn.Exec(
		"INSERT INTO tasks (id, content, deadline, created_at) VALUES (?, ?, ?, ?)",
		t.ID, t.Content, t.Deadline, t.CreatedAt,
	)
	return err
}

func (d *DB) GetTasks() ([]models.Task, error) {
	rows, err := d.conn.Query("SELECT id, content, deadline, created_at FROM tasks ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []models.Task
	for rows.Next() {
		var t models.Task
		rows.Scan(&t.ID, &t.Content, &t.Deadline, &t.CreatedAt)
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (d *DB) AddTaskFeedback(f *models.TaskFeedback) error {
	f.ID = generateUUID()
	f.FeedbackAt = time.Now()
	_, err := d.conn.Exec(
		"INSERT OR IGNORE INTO task_feedbacks (id, task_id, student_id, feedback_at) VALUES (?, ?, ?, ?)",
		f.ID, f.TaskID, f.StudentID, f.FeedbackAt,
	)
	return err
}

func (d *DB) GetTaskFeedbacks(taskID string) ([]models.TaskFeedback, error) {
	rows, err := d.conn.Query(
		"SELECT id, task_id, student_id, feedback_at FROM task_feedbacks WHERE task_id = ? ORDER BY feedback_at",
		taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var feedbacks []models.TaskFeedback
	for rows.Next() {
		var f models.TaskFeedback
		rows.Scan(&f.ID, &f.TaskID, &f.StudentID, &f.FeedbackAt)
		feedbacks = append(feedbacks, f)
	}
	return feedbacks, nil
}

func (d *DB) HasTaskFeedback(taskID, studentID string) (bool, error) {
	var count int
	err := d.conn.QueryRow(
		"SELECT COUNT(*) FROM task_feedbacks WHERE task_id = ? AND student_id = ?",
		taskID, studentID,
	).Scan(&count)
	return count > 0, err
}

// ---- 导出 ----

type ExportRecord struct {
	CallRecord
	Answerers []string `json:"answerers"`
	Askers    []string `json:"askers"`
}

func (d *DB) ExportCallRecords() ([]ExportRecord, error) {
	records, err := d.GetCallRecords()
	if err != nil {
		return nil, err
	}
	var result []ExportRecord
	for _, r := range records {
		rows, err := d.conn.Query(
			"SELECT student_id, role FROM call_details WHERE call_record_id = ? ORDER BY \"order\"",
			r.ID,
		)
		if err != nil {
			return nil, err
		}
		var answerers, askers []string
		for rows.Next() {
			var sid, role string
			rows.Scan(&sid, &role)
			if role == "answerer" {
				answerers = append(answerers, sid)
			} else {
				askers = append(askers, sid)
			}
		}
		rows.Close()
		result = append(result, ExportRecord{CallRecord: r, Answerers: answerers, Askers: askers})
	}
	return result, nil
}

func generateUUID() string {
	return fmt.Sprintf("%d-%d-%d-%d",
		time.Now().UnixNano(),
		rand.Int63(),
		time.Now().UnixNano()%10000,
		rand.Int63()%10000,
	)
}
