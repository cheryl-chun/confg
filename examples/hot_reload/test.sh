go run . &
PID=$!
sleep 1

# 修改 config.yaml，触发热更新
cat > config.yaml <<'EOF'
app:
  name: "my-service"
  log_level: "debug"
  debug: true

server:
  host: "0.0.0.0"
  port: 9090
  timeout_seconds: 60

feature:
  enable_cache: false
  max_cache_size: 256
EOF

sleep 1
kill $PID 2>/dev/null
wait $PID 2>/dev/null

# 恢复原始 config.yaml
cat > config.yaml <<'EOF'
app:
  name: "my-service"
  log_level: "info"
  debug: false

server:
  host: "0.0.0.0"
  port: 8080
  timeout_seconds: 30

feature:
  enable_cache: true
  max_cache_size: 512
EOF