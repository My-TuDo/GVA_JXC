# deploy/lightweight

阿里云轻量（Anolis 3，x86_64）原生部署所需文件。完整步骤见 [`docs/35-服务器部署与演示.md`](../../docs/35-服务器部署与演示.md)。

| 文件 | 用途 |
|---|---|
| `build.sh` | 本地构建 Linux amd64 二进制 + 前端 dist |
| `server-setup.sh` | 服务器一次性初始化（nginx/MySQL/用户/目录） |
| `upgrade.sh` | 服务器端替换产物并重启 |
| `reset-demo.sh` | 从 `golden.sql` 恢复演示库 |
| `nginx-jxc.conf` | 阶段一（回环 8080）与阶段二（80/443）模板 |
| `jxc.service` | systemd 单元（release、非 root、只读加固） |
| `my.cnf` | 1.8G 内存下的 MySQL 调优 |
| `sql/jxc-demo-roles.sql` | 9001/9002/9003 角色行（幂等） |

> **不要在服务器上 clone 仓库再构建**：1.8G 内存跑不动 `go build` / `vite build`，
> 且 `dist/` 产物被 `.gitignore` 排除，clone 下来也拿不到二进制，仍要单独传输。
> 服务器只接收产物与下列脚本，见下文 scp。

快速开始：

```bash
# 本地（Windows 用 Git Bash / WSL）
bash deploy/lightweight/build.sh
scp -r deploy/lightweight root@<IP>:/tmp/
scp server/config.example.yaml root@<IP>:/tmp/
scp docs/sql/jxc-casbin.sql scripts/seed-demo.py root@<IP>:/tmp/
ssh root@<IP> 'cd /tmp/lightweight && JXC_DB_PASSWORD=<密码> bash server-setup.sh'
```
