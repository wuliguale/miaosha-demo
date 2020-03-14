package controllers

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/mvc"
	"github.com/kataras/iris/v12/sessions"
	"github.com/streadway/amqp"
	"log"
	"miaosha-demo/common"
	"miaosha-demo/services"
	"strconv"
)

type ProductController struct {
	Ctx            iris.Context
	ProductService services.IProductService
	OrderService   services.IOrderService
	Session        *sessions.Session
}

//商品列表
func (p *ProductController) GetAll() mvc.View{
	page := p.Ctx.URLParamInt64Default("page", 1)
	pageSize := p.Ctx.URLParamInt64Default("pageSize", 10)

	productList, err := p.ProductService.GetProductAll(page, pageSize, "id", true)
	if err != nil {
		return errorReturnView(p.Ctx, err.Error(), "/", 500)
	}

	productListMap := common.StructPtrArray2MapArray(productList)

	for k,v := range productListMap {
		v["UrlDetail"] = "/product/one?id=" + fmt.Sprintf("%v", v["Id"])
		productListMap[k] = v
	}

	return mvc.View{
		Name: "product/list",
		Data: iris.Map{
			"productList": productListMap,
		},
	}
}


//一个商品的详情
func (p *ProductController) GetOne() mvc.View{
	idString := p.Ctx.URLParam("id")
	id, err := strconv.ParseInt(idString, 10, 16)
	if err != nil {
		return errorReturnView(p.Ctx, err.Error(), "/product/all", 500)
	}

	product, err := p.ProductService.GetProductByID(uint64(id))
	if err != nil {
		return errorReturnView(p.Ctx, err.Error(), "/product/all", 500)
	}

	return mvc.View{
		Name: "product/detail",
		Data: iris.Map{
			"product": common.StructPtr2Map(product),
		},
	}
}



func (p *ProductController) GetOrderInit() {
	pid := p.Ctx.URLParamInt64Default("pid", 0)
	if pid > 0 {
		//从连接池获取连接
		redisConn := RedisPool.Get()
		defer redisConn.Close()
	}
}


//秒杀接口
func (p *ProductController) GetOrder() {
	pid := p.Ctx.URLParamInt64Default("pid", 0)
	uid, err := services.GetUidFromCookie(p.Ctx)
	if pid == 0 || uid == 0 || err != nil {
		ReturnJsonFail(p.Ctx, "参数错误")
		return
	}

	//从连接池获取连接
	redisConn := RedisPool.Get()
	defer redisConn.Close()

	//秒杀是否已结束
	pidOverKey := "pid_over_" + strconv.Itoa(int(pid))
	isOver, err := redis.Int(redisConn.Do("get", pidOverKey))
	if err != nil  && err != redis.ErrNil{
		ReturnJsonFail(p.Ctx, "检查秒杀是否结束出错" + err.Error())
		return
	}
	if isOver > 0 {
		ReturnJsonFail(p.Ctx, "秒杀已结束")
		return
	}

	//是否重复购买
	isRepeatKey := "pid_" + strconv.Itoa(int(pid))
	isRepeat, err := redis.Int(redisConn.Do("getbit", isRepeatKey, uid))
	if err != nil && err != redis.ErrNil{
		ReturnJsonFail(p.Ctx, "检查是否重复购买出错" + err.Error())
		return
	}
	if isRepeat > 0 {
		ReturnJsonFail(p.Ctx, "不能重复购买")
		return
	}

	//检查库存
	numKey := "pid_num_" + strconv.Itoa(int(pid))
	num, err := redis.Int(redisConn.Do("decr", numKey))
	if err != nil && err != redis.ErrNil{
		ReturnJsonFail(p.Ctx, "redis检查库存错误" + err.Error())
		return
	}
	//这里判断小于0，等于0时当前连接获得左后一个
	if num < 0 {
		_, err = redis.String(redisConn.Do("set", pidOverKey, 1))
		if err != nil && err != redis.ErrNil {
			ReturnJsonFail(p.Ctx, "设置秒杀结束时错误" + err.Error())
			return
		}

		ReturnJsonFail(p.Ctx, "已无库存")
		return
	}

	ch, err := RabbitMqConn.Channel()
	if err != nil {
		ReturnJsonFail(p.Ctx, "rabbitmq channel错误")
		return
	}
	defer ch.Close()

	channelConfirm := ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	err = ch.Confirm(false)
	if err != nil {
		ReturnJsonFail(p.Ctx, "")
		return
	}

	body := strconv.Itoa(int(pid)) + "_" + strconv.Itoa(int(uid))
	err = ch.Publish(
		"miaosha_demo_exchange",
		"aaa.bbb.ccc",
		false,
		false,
		amqp.Publishing{
			ContentType:"text/plain",
			Body:[]byte(body),
		},
	)

	if err != nil {
		ReturnJsonFail(p.Ctx, "mq publish 错误")
		return
	}

	//标记用户已购买
	_, err = redis.Int(redisConn.Do("setbit", isRepeatKey, uid, 1))
	if err != nil && err != redis.ErrNil {
		ReturnJsonFail(p.Ctx, "标记用户已购买时出错")
		return
	}

	confirmRes := <-channelConfirm
	log.Printf("confirm: %v", confirmRes)

	data := map[string]interface{}{}
	ReturnJsonSuccess(p.Ctx, data)
	return
}

