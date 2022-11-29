#bin/bash

BINARY_NAME="probermesh"
DOWNLOAD_URL="https://github.com/resurgence72/ProberMesh/releases/download/v0.0.3/probermesh"
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
-server.rpc.addr=${SERVER_RPC_ADDR}
autostart = true
startsecs = 10
autorestart = true
startretries = 5
user = root
redirect_stderr = true
stdout_logfile_maxbytes = 100MB
stdout_logfile_backups = 10
stdout_logfile = /var/log/${BINARY_NAME}-agent.log
EOF

systemctl restart supervisord.service \
&& systemctl enable supervisord.service \
&& supervisorctl update \
&& supervisorctl start ${BINARY_NAME} \
&& supervisorctl status ${BINARY_NAME} \
&& echo "agent ${LOCAL_REGION} start success"
