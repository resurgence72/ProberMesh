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
Usage of ./probermesh:
   -agent.icmp.network-type string
        agentICMP探测agent自身上报网络类型(公网/内网) (default "intranet")
  -agent.probe.interval string
        agent端探测周期 (default "15s")
  -agent.region string
        agent端所属域;默认为region(自动获取regionID) (default "cn-shanghai")
  -agent.sync.interval string
        agent端同步targets周期 (default "1m")
  -agent.upgrade
        agent端是否开启自升级功能,默认关闭
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
scrape_configs:
  - job_name: "prober_mesh"
    static_configs:
      - targets: ["$server.http.listen.addr"]
    metric_relabel_configs:
      - source_labels: ["__name__"]
        regex: "^prober.*"
        action: 'keep'
```


#### Agent 节点自升级
##### 1. 需求及实现
```text
proberMesh 部署后存在一种场景，我们需要对 agent 角色节点进行升级或bugfix；通用的大批量agent升级操作方式无非如下几种
1. 手动人工替换二进制 restart
   弊端：操作繁琐,节点多了后需要耗费大量时间
2. ansible 自动化批量执行
   弊端: 节点多了后ssh执行效率很低
3. 基于 kubernetes 环境做 Deployment upgrade
   弊端: proberMesh 大部分场景是部署在同内网的多region, 或者不同内网的region通过公网通讯；使用k8s托管成本太高
4. self upgrade节点自升级
   弊端: 代码层面适配，大规模节点下升级有可能把管理机带宽打满(proberMesh主要多对多拨测，节点数量不会很大，打满问题不太会发生)


当前使用方法4实现了自升级，这种实现方式有个前提: agent 节点需要使用守护进程托管，例如 supervisord; 

升级流程： 管理机向 server 端 http://${server_http_addr}/upgrade 发送 POST 升级请求：
{
"downloadURL": "http://172.18.12.38:9999/probermesh",
"md5Check": "0a85983226d91029bcf5701f94d18753",
"version": "0.0.1",
"force": false,
}
downloadURL 标识agent新版本二进制的下载地址，这里要注意，二进制名称要和原本一直，否则守护进程会restart失败;
md5Check 标识agent新版本的md5，agent需要和下载的二进制做md5校验，校验成功才会替换升级;
version 标识新版本的version;
force 标识是否强制升级; false情况下，agent只校验version > 本身的升级请求；true情况下，agent跳过version check,其实也就是支持回滚;

agent 定时向server端获取upgrade信息，拿到后check成功会在本地下载并替换二进制并且 kill self, 后续再被守护进程拉起时即是新版本；升级期间建议通过 agent_is_alive{version="0.0.1"} 指标监控升级情况；
```
##### 2. 存在的问题
```text
如果做灰度或做并发控制？ 这种需求通常存在于 agent 节点规模很大的场景
1. 需要一批批替换，例如两千个agent, 100台100台的升级；
2. 管理机存在其他业务，需要限制带宽被打满

由于proberMesh暂时无此需求，我这里提供一个思路，后续有需求时再实现：
通过redis分布式锁控制set升级队列
```


#### 最佳实践
```text
ICMP网格需求下:
- 内网互通环境下，使用server动态模式 -server.icmp.discovery=dynamic + agent内网模式 -agent.icmp.netowrk-type=intranet 拿到内网，以内网作为ip池上报server;
- 公网环境下，使用server动态模式 -server.icmp.discovery=dynamic + agent公网模式 -agent.icmp.netowrk-type=public 拿到公网ip，以公网网作为ip池上报server;

其余需求下:
使用server静态模式 -server.icmp.discovery=static + agent指定配置文件 -server.probe.file=./prober_mesh.yaml 自定义互(ping/http)对象
```
