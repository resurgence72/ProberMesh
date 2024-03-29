package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"probermesh/pkg/config"
	"probermesh/pkg/util"

	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

type ProberMeshServerOption struct {
	TargetsConfigPath   string
	ICMPDiscoveryType   string
	HTTPListenAddr      string
	RPCListenAddr       string
	AggregationInterval string
	TaskMetaDir         string

	SeriesCacheRatio int

	TaskEnabled bool
	ProbeSelf   bool
}

type reloader struct {
	name     string
	reloader func() error
}

func BuildServerMode(so *ProberMeshServerOption) {
	verify := func() error {
		if discoveryType(so.ICMPDiscoveryType) == StaticDiscovery && len(so.TargetsConfigPath) == 0 {
			// static 模式下配置文件必须指定，否则icmp没数据，http也没数据；无意义
			return errors.New("flag -server.probe.file must be set when -server.icmp.discovery flag is 'static'")
		}

		// 动态模式下配置参数可以不指定，有icmp保底
		if len(so.TargetsConfigPath) > 0 {
			if err := config.InitConfig(so.TargetsConfigPath); err != nil {
				logrus.Errorln("server parse config failed ", err)
				return err
			}
		}

		return nil
	}
	if err := verify(); err != nil {
		logrus.Fatalln("server config verify failed ", err)
	}

	ctxAll, cancelAll := context.WithCancel(context.Background())
	defer cancelAll()

	// 解析agg interval
	aggD, err := util.ParseDuration(so.AggregationInterval)
	if err != nil {
		logrus.Errorln("agg interval parse failed ", err)
		return
	}

	// rpc server
	if err := startRpcServer(so.RPCListenAddr); err != nil {
		logrus.Fatalln("start rpc server failed ", err)
		return
	}

	var (
		g        run.Group
		reloadCh = make(chan chan error)

		// 首次上报后 再updatePool,否则update不到数据
		ready = make(chan struct{})
	)

	pool := newTargetsPool(
		ctxAll,
		config.Get,
		reloadCh,
		ready,
		so.ICMPDiscoveryType,
		so.ProbeSelf,
	)
	reloaders := []reloader{
		{
			name:     "config",
			reloader: config.ReloadConfig,
		},
		{
			name:     "pool",
			reloader: pool.reloadPool,
		},
	}
	{
		if so.ProbeSelf {
			proberMeshServerProbeSelfEnabledGauge.Set(1)
		} else {
			proberMeshServerProbeSelfEnabledGauge.Set(0)
		}
		// 初始化targetsPool
		g.Add(func() error {
			pool.start()
			return nil
		}, func(err error) {
			cancelAll()
		})
	}

	{
		// health check 打点
		g.Add(func() error {
			newHealthDot(
				ctxAll,
				aggD,
				so.SeriesCacheRatio,
				ready,
			).dot()
			return nil
		}, func(e error) {
			cancelAll()
		})
	}

	{
		// aggregation
		g.Add(func() error {
			newAggregator(
				ctxAll,
				aggD,
				so.SeriesCacheRatio,
			).startAggregation()
			return nil
		}, func(err error) {
			cancelAll()
		})
	}

	{
		tg := newTaskGroup(so.TaskEnabled, so.TaskMetaDir)
		// http server
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		mux.HandleFunc("/-/upgrade", util.WithJSONHeader(update))
		mux.HandleFunc("/-/reload", func(w http.ResponseWriter, r *http.Request) {
			ch := make(chan error)
			reloadCh <- ch
			if err = <-ch; err != nil {
				_, _ = w.Write([]byte(err.Error()))
			} else {
				_, _ = w.Write([]byte("ok"))
			}
		})
		if so.TaskEnabled {
			logrus.Warnln("open server task dispatch functions")
			proberMeshServerTaskEnabledGauge.Set(1)
			mux.HandleFunc("/-/task", util.WithJSONHeader(tg.task))
		} else {
			proberMeshServerTaskEnabledGauge.Set(0)
			mux.HandleFunc("/-/task", util.WithJSONHeader(func(r *http.Request) []byte {
				bs, _ := json.Marshal(map[string]interface{}{
					"code": -1,
					"msg":  "error",
					"data": "Interface not open, use -server.task to open func",
				})
				return bs
			}))
		}

		mux.HandleFunc("/-/targets", util.WithJSONHeader(func(r *http.Request) []byte {
			bs, _ := json.Marshal(tp.getTargets())
			return bs
		}))
		svc := http.Server{
			Addr:    so.HTTPListenAddr,
			Handler: mux,
		}

		errCh := make(chan error)
		go func() {
			errCh <- svc.ListenAndServe()
		}()

		g.Add(func() error {
			select {
			case <-errCh:
			case <-ctxAll.Done():
			}
			return svc.Shutdown(context.TODO())
		}, func(err error) {
			cancelAll()
		})
	}

	{
		// reload
		g.Add(func() error {
			for {
				select {
				case ch := <-reloadCh:
					ch <- reloadConfig(reloaders)
				case <-ctxAll.Done():
					return nil
				}
			}
		}, func(e error) {
			logrus.Warnln("reload manager over")
			cancelAll()
		})
	}

	{
		// 信号管理
		term := make(chan os.Signal, 1)
		signal.Notify(term, os.Interrupt, syscall.SIGTERM)
		cancel := make(chan struct{})

		g.Add(func() error {
			select {
			case <-term:
				logrus.Warnln("优雅关闭ing...")
				cancelAll()
				return nil
			case <-cancel:
				return nil
			}
		}, func(err error) {
			close(cancel)
			logrus.Warnln("signal controller over")
		})
	}

	g.Run()
}

func reloadConfig(reloaders []reloader) error {
	logrus.Warnln("reloaders start reload")

	failed := false
	for _, reloader := range reloaders {
		if err := reloader.reloader(); err != nil {
			failed = true

			logrus.WithFields(logrus.Fields{
				"status":   "failed",
				"reloader": reloader.name,
			}).Errorln("reloader failed")
		}
	}

	if failed {
		return errors.New("one or more reloader reload failed")
	}
	return nil
}
