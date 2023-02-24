package main

import (
	"flag"
	"fmt"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"path"
	"probermesh/pkg/agent"
	"probermesh/pkg/server"
	"probermesh/pkg/util"
	"time"
)

type config struct {
	mode     string
	logDir   string
	logLevel int
	h        bool
	v        bool
}

var (
	serverOption = new(server.ProberMeshServerOption)
	agentOption  = new(agent.ProberMeshAgentOption)

	cfg = new(config)
)

func initLog(c *config) {
	var (
		logDir = c.logDir
		// 日志级别
		logLevel = logrus.Level(c.logLevel)
		// 日志格式
		format = &logrus.TextFormatter{TimestampFormat: "2006-01-02 15:04:05"}

		// 日志文件根目录
		logName = util.ProjectName + "-" + c.mode
		logPath = path.Join(logDir, fmt.Sprintf("%s.log", logName))
	)

	// 持久化日志
	if _, err := os.Stat(logDir); err != nil && os.IsNotExist(err) {
		if err = os.Mkdir(logDir, 0666); err != nil {
			logrus.WithFields(logrus.Fields{
				"err": err,
			}).Fatalln("can not mkdir:", logDir)
		}
	}

	// 创建log文件
	file, err := os.OpenFile(
		logPath,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0666,
	)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"err": err,
		}).Fatalln("can not find logs dir or file")
	}
	writers := []io.Writer{file, os.Stdout}
	multiWriters := io.MultiWriter(writers...)

	// 同时写文件和屏幕
	logrus.SetOutput(multiWriters)

	// 配置log分割
	logf, err := rotatelogs.New(
		// 切割的日志名称
		path.Join(logDir, logName)+"-%Y%m%d.log",
		// 日志软链
		rotatelogs.WithLinkName(logPath),
		// 日志最大保存时长
		rotatelogs.WithMaxAge(time.Duration(7*24)*time.Hour),
		// 日志切割时长
		rotatelogs.WithRotationTime(time.Duration(24)*time.Hour),
	)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"err": err,
		}).Fatalln("can not new rotatelog")
	}

	// 添加logrus钩子
	hook := lfshook.NewHook(
		lfshook.WriterMap{
			logrus.InfoLevel:  logf,
			logrus.FatalLevel: logf,
			logrus.DebugLevel: logf,
			logrus.WarnLevel:  logf,
			logrus.ErrorLevel: logf,
			logrus.PanicLevel: logf,
		},
		format,
	)
	logrus.AddHook(hook)

	// 设置日志记录级别
	logrus.SetLevel(logLevel)
	// 输出日志中添加文件名和方法信息
	logrus.SetReportCaller(true)
	// 设置日志格式
	logrus.SetFormatter(format)
}

func initArgs() {
	flag.StringVar(&cfg.mode, "mode", "server", "服务模式(agent/server)")
	flag.StringVar(&cfg.logDir, "log.dir", "/logs/", "日志目录")
	flag.IntVar(&cfg.logLevel, "log.level", 3, `日志级别;
PanicLevel 0
FatalLevel 1
ErrorLevel 2
WarnLevel  3
InfoLevel  4
DebugLevel 5
TraceLevel 6
`)
	flag.BoolVar(&cfg.v, "v", false, "版本信息")
	flag.BoolVar(&cfg.h, "h", false, "帮助信息")

	flag.StringVar(&serverOption.TargetsConfigPath, "server.probe.file", "", "server端探测列表文件路径")
	flag.StringVar(&serverOption.ICMPDiscoveryType, "server.icmp.discovery", "dynamic", `server端ICMP探测目标获取模式(static/dynamic);
static:  各region下icmp探测地址按照配置文件为准;
dynamic: 各region下icmp探测地址按照agent自上报服务发现为准，且会覆盖掉配置中同region下原icmp列表;
`)
	flag.StringVar(&serverOption.RPCListenAddr, "server.rpc.listen.addr", "localhost:6000", "serverRPC监听地址")
	flag.StringVar(&serverOption.HTTPListenAddr, "server.http.listen.addr", "localhost:6001", "serverHTTP监听地址")
	flag.StringVar(&serverOption.AggregationInterval, "server.aggregation.interval", "15s", "server聚合周期")
	flag.StringVar(&serverOption.TaskMetaDir, "server.task.meta.dir", "./task_meta/", "server持久化task结果源目录")
	flag.BoolVar(&serverOption.TaskEnabled, "server.task", false, "server是否开启task任务下发功能")
	flag.BoolVar(&serverOption.ProbeSelf, "server.probe.self", false, "server控制agent是否允许同region拨测") // 默认为false,agent只会拿到非自身region的拨测列表
	flag.IntVar(&serverOption.SeriesCacheRatio, "server.series.cache.ratio", 5, "server指标缓存时长重置倍率") // 缓存时长倍率，超过 cacheDurationRatio*interval 的series会被清除掉

	flag.StringVar(&agentOption.ReportAddr, "agent.rpc.report.addr", "localhost:6000", "server端RPC上报地址")
	flag.StringVar(&agentOption.PInterval, "agent.probe.interval", "15s", "agent端探测周期")
	flag.StringVar(&agentOption.SInterval, "agent.sync.interval", "1m", "agent端同步targets周期")
	flag.StringVar(&agentOption.Region, "agent.region", "china-shanghai", "agent端所属region/zone;不指定默认自动获取regionID")
	flag.StringVar(&agentOption.UInterval, "agent.upgrade.interval", "1m", "agent端检查upgrade周期; 仅在指定 -agent.upgrade 后生效")
	flag.StringVar(&agentOption.NetworkType, "agent.icmp.network-type", "intranet", `agent ICMP探测agent自身上报IP类型;
intranet: agent上报内网IP地址，用与构建内网维度icmp网格；
public: agent上报公网IP地址，用于构建公网维度下icmp网格；
`)
	flag.BoolVar(&agentOption.Upgrade, "agent.upgrade", false, "agent端是否开启自升级功能,默认关闭")

	flag.Parse()

	if cfg.v {
		logrus.WithField("version", util.GetVersion()).Println("version")
		os.Exit(0)
	}

	if cfg.h {
		flag.Usage()
		os.Exit(0)
	}
}

func main() {
	initArgs()

	switch cfg.mode {
	case "agent":
		initLog(cfg)
		logrus.Warnln("build for agent mode")
		agent.BuildAgentMode(agentOption)
	case "server":
		initLog(cfg)
		logrus.Warnln("build for server mode")
		server.BuildServerMode(serverOption)
	default:
		logrus.Fatal("mode must be set and in agent/server")
	}
}
