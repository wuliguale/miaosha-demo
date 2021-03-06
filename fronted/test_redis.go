package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/go-redis/redis/v7"
	"go.uber.org/zap"
	"miaosha-demo/common"
	"time"
)

/**
测试redis的处理时间
 */
func main() {
	common.NewZapLogger()

	defer func() {
		//recover panic, only for unexpect exception
		r := recover()

		if r != nil {
			var rerr error
			switch e := r.(type) {
			case string:
				rerr = errors.New(e)
			case error:
				rerr = e
			default:
				rerr = errors.New(fmt.Sprintf("%v", e))
			}
			common.ZapError("recover error", rerr)
		}

		zap.L().Sync()
	} ()

	flagTotal := flag.Int("total", 100, "transaction total")
	flag.Parse()
	total := *flagTotal

	config, err := common.NewConfigConsul()
	if err != nil {
		common.ZapError("new config consul fail", err)
		return
	}
	cache := common.NewFreeCacheClient(20)

	Consul, err := common.NewConsulClient(config, cache)
	if err != nil {
		common.ZapError("new consul client fail", err)
		return
	}

	//取consul上redis service的配置
	redisClusterClient, err := common.NewRedisClusterClient(Consul)
	if err != nil {
		common.ZapError("new redis cluster fail", err)
		return
	}

	pid := 1
	uid := 1
	timeStart := time.Now()

	for i := 0; i < total; i++ {
		uid += i

		pidOverKey := fmt.Sprintf("pid_over_%d", pid)
		isOver, err := redisClusterClient.Get(pidOverKey).Int()
		if err != nil  && err != redis.Nil {
			common.ZapError("检查秒杀是否结束出错", err)
			continue
		}

		if isOver > 0 {
			zap.L().Info("秒杀已结束")
			continue
		}

		//是否已参加过
		isRepeatKey := fmt.Sprintf("pid_uid_repeat_%d_%d",  pid, uid)
		isRepeat, err := redisClusterClient.Incr(isRepeatKey).Result()
		if err != nil && err != redis.Nil {
			common.ZapError("检查是否重复参加出错", err)
			continue
		}
		if isRepeat > 1 {
			zap.L().Info("不能重复参加")
			continue
		}

		//检查库存
		numKey := fmt.Sprintf("pid_num_%d", pid)
		num, err := redisClusterClient.Decr(numKey).Result()
		if err != nil && err != redis.Nil {
			common.ZapError("redis检查库存错误", err)
			continue
		}

		//这里判断小于0，等于0时当前连接获得最后一个
		if num < 0 {
			err = redisClusterClient.Set(pidOverKey, 1, 0).Err()
			if err != nil && err != redis.Nil {
				common.ZapError("设置秒杀结束时错误", err)
				continue
			}

			zap.L().Info("已无库存")
			continue
		}
	}
	
	timeEnd := time.Now()
	timeTotal := timeEnd.Sub(timeStart).Milliseconds()
	timeAvg := timeTotal/ int64(total)
	fmt.Println(fmt.Sprintf("transactionNum: %d,timeTotal: %d ms, timeAvg: %d ms", total, timeTotal, timeAvg))
}

