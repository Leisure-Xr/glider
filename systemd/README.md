## 服务

### 安装步骤

#### 1. 复制可执行文件

```bash
cp glider /usr/bin/
```

#### 2. 添加服务文件

```bash
# 将服务文件复制到 systemd 目录
cp systemd/glider@.service /etc/systemd/system/
```

#### 3. 添加配置文件：***glider***.conf

```bash
# 将配置文件复制到 /etc/glider/
mkdir /etc/glider/
cp ./config/glider.conf.example /etc/glider/glider.conf
```

#### 4. 启用并启动服务：glider@***glider***

```bash
# 启用并启动服务
systemctl enable glider@glider
systemctl start glider@glider
```

参考 [glider@.service](glider%40.service)
