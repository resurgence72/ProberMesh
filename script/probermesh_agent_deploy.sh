#bin/bash

BINARY_NAME="probermesh"
DOWNLOAD_URL="https://github.com/resurgence72/ProberMesh/releases/download/v0.0.5/probermesh" # 推荐使用最新版本
SERVER_RPC_ADDR="1.1.1.1:6000"
LOCAL_REGION=$1


cd /root \
&& yum -y install wget epel-release supervisor \
&& wget -O ${BINARY_NAME} ${DOWNLOAD_URL} \
&& chmod +x ${BINARY_NAME}

cat > /etc/supervisord.d/${BINARY_NAME}.ini << EOF
[program:${BINARY_NAME}]
directory = /root
command = /root/${BINARY_NAME} -mode agent \
-agent.probe.interval=15s \
-agent.icmp.network-type=public \
-agent.region=${LOCAL_REGION}  \
-agent.rpc.report.addr=${SERVER_RPC_ADDR}
autostart = true
startsecs = 10
autorestart = true
startretries = 5
user = root
redirect_stderr = true
EOF

systemctl restart supervisord.service \
&& systemctl enable supervisord.service \
&& supervisorctl update \
&& supervisorctl restart ${BINARY_NAME} \
&& supervisorctl status ${BINARY_NAME} \
&& echo "agent ${LOCAL_REGION} deploy and start success"
