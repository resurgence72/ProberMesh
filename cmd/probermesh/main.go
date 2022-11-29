package main

import (
	"flag"
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

const projectName = "ProberMesh"

var (
	serverOption = new(server.ProberMeshServerOption)
	agentOption  = new(agent.ProberMeshAgentOption)

	mode string
	h    bool
	v    bool
)

func init() {
	initArgs()
	initLog()
}

func initLog() {
	var (
		// 日志级别
		logLevel = logrus.DebugLevel
		// 日志格式
		format = &logrus.TextFormatter{TimestampFormat: "2006-01-02 15:04:05"}

		// 日志文件根目录
		logDir  = "/logs/"
		logName = projectName
		logPath = path.Join(logDir, logName+".log")
	)

	// prod 环境提高日志等级
	logLevel = logrus.WarnLevel

	// 持久化日志
	if _, err := os.Stat(logDir); err != nil && os.IsNotExist(err) {
		if err := os.Mkdir(logDir, 0666); err != nil {
			logrus.WithFields(logrus.Fields{
				"err": err,
			}).Fatalln("can not mkdir logs")
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

	// 同时记录gin的日志
	//gin.DefaultWriter = multiWriters
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
	flag.StringVar(&mode, "mode", "server", "服务模式(agent/server)")

	flag.StringVar(&serverOption.TargetsConfigPath, "server.probe.file", "", "server端探测列表文件路径")
	flag.StringVar(&serverOption.ICMPDiscoveryType, "server.icmp.discovery", "dynamic", `server端ICMP探测目标获取模式(static/dynamic);
static:  各region下icmp探测地址按照配置文件为准;
dynamic: 各region下icmp探测地址按照agent自上报服务发现为准，且会覆盖掉配置中同region下原icmp列表;
`)
	flag.StringVar(&serverOption.RPCListenAddr, "server.rpc.listen.addr", "localhost:6000", "serverRPC监听地址")
	flag.StringVar(&serverOption.HTTPListenAddr, "server.http.listen.addr", "localhost:6001", "serverHTTP监听地址")
	flag.StringVar(&serverOption.AggregationInterval, "server.aggregation.interval", "15s", "server聚合周期")

	flag.StringVar(&agentOption.Addr, "server.rpc.addr", "localhost:6000", "server端RPC地址")
	flag.StringVar(&agentOption.PInterval, "agent.probe.interval", "15s", "agent端探测周期")
	flag.StringVar(&agentOption.SInterval, "agent.sync.interval", "1m", "agent端同步targets周期")
	flag.StringVar(&agentOption.Region, "agent.region", "cn-shanghai", "agent端所属域;默认为region(自动获取regionID)")
	flag.BoolVar(&agentOption.Upgrade, "agent.upgrade", false, "agent端是否开启自升级功能,默认关闭")
	flag.BoolVar(&v, "v", false, "版本信息")
	flag.BoolVar(&h, "h", false, "帮助信息")
	flag.Parse()

	if v {
		logrus.WithField("version", util.GetVersion()).Println("version")
		os.Exit(0)
	}

	if h {
		flag.Usage()
		os.Exit(0)
	}
}

func main() {
	switch mode {
	case "agent":
		logrus.Warnln("build for agent mode")
		agent.BuildAgentMode(agentOption)
	case "server":
		logrus.Warnln("build for server mode")
		server.BuildServerMode(serverOption)
	default:
		logrus.Fatal("mode must be set and in agent/server")
	}
}
