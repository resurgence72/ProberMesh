# ProberMesh
#### 分布式 C/S 网络网格探测框架

> ICMP 视野落在 Region to Region ；
>
> HTTP 视野落在 Region to URL ；
>
> 同 Region 下做 agg；
>
>[Region的意义可以自定义，例如(K8s环境/Host环境/Region/各云厂商等)]

- ##### 支持 ICMP/HTTP 分阶段耗时

- ##### ICMP 支持 resolve/rtt 分阶段耗时

- ##### ICMP 支持丢包率， 抖动标准差

- ##### HTTP 支持 resolve/connect/tls/processing/transfer 分阶段耗时 (httpstat)

#### 

```golang
// ping 指标
prober_icmp_failed
prober_icmp_duration_seconds
prober_icmp_packet_loss_rate
prober_icmp_jitter_stddev_seconds
prober_icmp_duration_seconds_total (histogram)

// http 指标
prober_http_failed
prober_http_duration_seconds

// 健康检查
prober_agent_is_alive
```

#### 运维指南
```shell
# 项目构建
cd ProberMesh/cmd/proberMesh && go build -o proberMesh .

# 项目运行
./proberMesh -h
  -agent.probe.interval string
        agent端探测周期 (default "15s")
  -agent.region string
        agent端所属域;默认为region(自动获取regionID)
  -agent.sync.interval string
        agent端同步targets周期 (default "1m")
  -h    帮助信息
  -mode string
        服务模式(agent/server) (default "server")
  -server.aggregation.interval string
        server聚合周期 (default "15s")
  -server.http.listen.addr string
        serverHTTP监听地址 (default "localhost:6001")
  -server.icmp.discovery string
        server端ICMP探测目标获取模式(static/dynamic);
        static:  各region下icmp探测地址按照配置文件为准;
        dynamic: 各region下icmp探测地址按照agent自上报服务发现为准，且会覆盖掉配置中同region下原icmp列表;
         (default "dynamic")
  -server.probe.file string
        server端探测列表文件路径
  -server.rpc.addr string
        server端RPC地址 (default "localhost:6000")
  -server.rpc.listen.addr string
        serverRPC监听地址 (default "localhost:6000")
  -v    版本信息


# 探测列表文件(可选)
cat prober_mesh.yaml
prober_configs:
  - prober_type: http
    region: ali-cn-beijing
    targets:
      - http://www.baidu.com
      - http://www.taobao.com
  - prober_type: icmp
    region: gcp-ap-southeast-1
    targets:
      - 8.8.8.8
      - www.baidu.com



# server 端使用
./proberMesh


# agent 端使用
./proberMesh -mode agent -agent.region ali-cn-shanghai -server.rpc.addr localhost:6000

PS: region参数优先级
1. 命令行 -agent.region
2. 环境变量 PROBER_REGION
3. 代码curl云厂商region接口(仅支持aliyun)
4. 使用默认region cn-shanghai

其余参数可根据需要自定义调整


PS: -server.icmp.discovery 参数支持两种icmp发现模式
1. static静态发现:  icmp探测地址从静态配置文件中获取，需指定 -server.probe.file 参数配合使用，否则无icmp和http数据，启动没意义
2. dynamic动态发现: icmp探测地址由agent动态上报，并且agent 每1m (-agent.sync.interval) 向server同步一次，可自动生成一张icmp Mesh 网格

http参数仅支持配置文件指定
```

#### Prometheus 端配置拉取
```shell script
global:
  scrape_interval: 15s
scrape_configs:
  - job_name: "prober_mesh"
    static_configs:
      - targets: ["$server.http.listen.addr"]
    metric_relabel_configs:
      - source_labels: ["__name__"]
        regex: "^prober.*"
        action: 'keep'
```
