package controllers

import (
	"github.com/garyburd/redigo/redis"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/mvc"
	"github.com/streadway/amqp"
	"miaosha-demo/common"
)


const RETURN_SUCCESS  = 0
const RETURN_FAIL  = 1

//controller包内初始化redis连接池
var RedisPool *redis.Pool
//初始化rabbitmq连接
var RabbitMqConn *amqp.Connection
func init() {
	RedisPool = common.NewRedisPool()
	RabbitMqConn, _ = common.NewRabbitMqConn()
}


//显示错误
func errorReturnView(ctx iris.Context, msg, url string, statusCode int) mvc.View {
	ctx.StatusCode(statusCode)

	return mvc.View{
		Name: "error.html",
		Data: iris.Map{
			"msg" : msg,
			"url" : url,
		},
	}
}

//提示信息并跳转
func messageThenRedirect(msg, url string) mvc.View {
	return mvc.View {
		Name : "message.html",
		Data : iris.Map {
			"msg" : msg,
			"url" : url,
		},
	}
}

//接口成功，返回数据
func ReturnJsonSuccess(ctx iris.Context, data map[string]interface{}) {
	ctx.JSON(iris.Map{
		"code":  RETURN_SUCCESS,
		"data" : data,
	})
}


//接口失败，返回错误信息
func ReturnJsonFail(ctx iris.Context, msg string) {
	ctx.JSON(iris.Map {
		"code" : RETURN_FAIL,
		"msg" : msg,
	})
}
