# 智慧课堂点名提问系统

## 构建说明

### macOS

1. 确保已安装 Python 3.8+（推荐从 python.org 安装）

2. 打开终端，进入项目目录：
   ```bash
   cd rollcall-pro
   ```

3. 运行构建脚本：
   ```bash
   chmod +x build_mac.sh
   ./build_mac.sh
   ```

4. 构建完成后，运行：
   ```bash
   open dist/rollcall-mac/rollcall.app
   ```

### Windows

1. 确保已安装 Python 3.8+（从 python.org 安装时勾选 "Add Python to PATH"）

2. 打开文件资源管理器，进入项目目录

3. 双击运行 `build_windows.bat`

4. 构建完成后，运行：
   ```
   dist\rollcall\rollcall.exe
   ```

### 直接运行（不打包）

如果不想打包，也可以直接运行：

```bash
# 安装依赖
pip install -r requirements.txt

# 运行
python rollcall.py

# 访问
# 教师端: http://localhost:8080/teacher
# 学生端: http://localhost:8080/student
```

## 功能

- 学生管理（增删改查、批量导入）
- 两种点名模式（多人问答、多对一）
- 冷却机制（回答者3天冷却）
- 倒计时控制
- 课堂任务发布
- 题库管理
- AI出题（需配置API）
- CSV导出

## 配置

- 数据库：运行后自动创建 `data/rollcall.db`
- 端口：默认8080，可通过命令行参数修改：
  ```bash
  python rollcall.py --port 9000
  ```
