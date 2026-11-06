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

快速开始：

```bash
bash deploy/lightweight/build.sh
scp -r deploy/lightweight root@<IP>:/tmp/
ssh root@<IP> 'cd /tmp/lightweight && JXC_DB_PASSWORD=<密码> bash server-setup.sh'
```
