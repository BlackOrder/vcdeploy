# Troubleshooting

Common issues and their solutions.

## Connection Issues

### Agent Cannot Connect to Master

**Symptoms:**
- Agent logs show connection errors
- No heartbeats in master logs

**Checks:**
1. Verify master address is correct:
   ```bash
   # Agent config
   master_address: "master.example.com:9001"
   ```

2. Check network connectivity:
   ```bash
   telnet master.example.com 9001
   # or
   nc -zv master.example.com 9001
   ```

3. Verify firewall rules allow port 9001

4. Check TLS configuration:
   ```bash
   openssl s_client -connect master.example.com:9001
   ```

5. Verify token is correct:
   ```bash
   vcdeploy agent token create my-agent
   # Use the generated token in agent config
   ```

### Agent Disconnects Frequently

**Symptoms:**
- Agent shows reconnection messages
- Deployments interrupted

**Solutions:**
1. Check network stability
2. Increase heartbeat timeout:
   ```yaml
   connection:
     heartbeat_interval: "60s"
   ```
3. Check for resource exhaustion on agent

## Deployment Failures

### "Repository not found"

**Cause:** Cannot access Git repository

**Solutions:**
1. Verify repository URL is correct
2. Check SSH key is configured:
   ```bash
   vcdeploy secret set DEPLOY_KEY "$(cat ~/.ssh/deploy_key)"
   ```
3. For HTTPS, verify credentials:
   ```bash
   vcdeploy secret set GIT_TOKEN "your-token"
   ```

### "Permission denied" During Deployment

**Cause:** Agent lacks filesystem permissions

**Solutions:**
1. Check deployment path ownership:
   ```bash
   ls -la /var/www/myapp
   ```
2. Ensure agent runs as correct user
3. Verify shared directories exist with correct permissions

### Deployment Stuck

**Symptoms:**
- Deployment shows "running" for too long
- No progress in logs

**Solutions:**
1. Check deployment logs:
   ```bash
   vcdeploy deploy logs <deployment-id>
   ```
2. Check agent logs:
   ```bash
   journalctl -u vcdeploy-agent -f
   ```
3. Cancel and retry:
   ```bash
   vcdeploy deploy cancel <deployment-id>
   vcdeploy deploy trigger myapp
   ```

### Symlink Not Updated

**Cause:** Atomic symlink operation failed

**Solutions:**
1. Check target filesystem supports symlinks
2. Verify permissions on parent directory
3. Check disk space

## Authentication Issues

### "Invalid token"

**Solutions:**
1. Regenerate API token:
   ```bash
   vcdeploy apikey create --description "My token"
   ```
2. Check token hasn't expired
3. Verify token is being sent correctly:
   ```bash
   curl -H "Authorization: Bearer <token>" http://localhost:9000/api/v1/health
   ```

### "TOTP required"

**Solution:** User has 2FA enabled. Provide TOTP code during login.

### Session Expired

**Solution:** Re-authenticate:
```bash
vcdeploy login
```

## Database Issues

### "Database is locked"

**Cause:** SQLite concurrent access issue

**Solutions:**
1. Check for long-running queries
2. Increase busy timeout:
   ```yaml
   database:
     busy_timeout: "30s"
   ```
3. Ensure only one master process is running

### Database Corruption

**Solutions:**
1. Stop master server
2. Backup current database:
   ```bash
   cp /var/lib/vcdeploy/vcdeploy.db /var/lib/vcdeploy/vcdeploy.db.backup
   ```
3. Attempt recovery:
   ```bash
   sqlite3 /var/lib/vcdeploy/vcdeploy.db "PRAGMA integrity_check;"
   ```
4. If corrupted, restore from backup

## Performance Issues

### High Memory Usage

**Solutions:**
1. Check for memory leaks in deployment scripts
2. Limit concurrent deployments
3. Reduce log retention

### Slow API Responses

**Solutions:**
1. Enable query logging:
   ```yaml
   logging:
     level: "debug"
   ```
2. Check database size and vacuum:
   ```bash
   sqlite3 /var/lib/vcdeploy/vcdeploy.db "VACUUM;"
   ```
3. Review slow queries in logs

## TLS Issues

### Certificate Expired

**Solutions:**
1. Check certificate expiry:
   ```bash
   openssl x509 -enddate -noout -in /etc/vcdeploy/tls/server.crt
   ```
2. Renew certificate
3. Restart master server

### Certificate Not Trusted

**Solutions:**
1. Add CA to system trust store
2. Or configure agent to trust CA:
   ```yaml
   tls:
     ca_file: "/etc/vcdeploy/ca.crt"
   ```

## Getting Help

If issues persist:

1. **Collect logs:**
   ```bash
   journalctl -u vcdeploy --since "1 hour ago" > master-logs.txt
   journalctl -u vcdeploy-agent --since "1 hour ago" > agent-logs.txt
   ```

2. **Check version:**
   ```bash
   vcdeploy version
   ```

3. **Review documentation:**
   - [GitHub Issues](https://github.com/BlackOrder/vcdeploy/issues)
   - [API Reference](api.md)

4. **Open an issue** with:
   - vcdeploy version
   - OS and architecture
   - Relevant log excerpts
   - Steps to reproduce
