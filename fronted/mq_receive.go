package main

import (
	"github.com/streadway/amqp"
	"log"
	"miaosha-demo/common"
	"miaosha-demo/datamodels"
	"miaosha-demo/repositories"
	"miaosha-demo/services"
	"strconv"
	"strings"
	"time"
)

func main() {
	config, err := common.NewConfigConsul()
	if err != nil {
		common.ZapError("new config fail", err)
		return
	}

	freeCache := common.NewFreeCacheClient(10)
	consulClient, err := common.NewConsulClient(config, freeCache)
	if err != nil {
		common.ZapError("new consul fail", err)
		return
	}

	//连接db
	mysqlPoolProduct, err := common.NewMysqlPoolProduct(consulClient)
	if err != nil {
		common.ZapError("new mysql pool product fail", err)
		return
	}

	rabbitmqPool, err := common.NewRabbitmqPool(consulClient)
	if err != nil {
		common.ZapError("new rabbitmq pool fail", err)
		return
	}

	conn, err := rabbitmqPool.Get()
	//conn, err := amqp.Dial("amqp://root:root@172.18.0.99/:5672/")
	defer conn.Close()
	if err != nil {
		common.ZapError("rabbitmq get fail", err)
		return
	}

	ch, err := conn.Channel()
	defer ch.Close()
	if err != nil {
		common.ZapError("rabbitmq new channel fail", err)
		return
	}

	//声明交换器，并指定备份交换器
	argTable := amqp.Table{"alternate-exchange":"miaosha_demo_exchange_ae"}
	err = ch.ExchangeDeclare(
		"miaosha_demo_exchange",
		"topic",
		true,
		false,
		false,
		false,
		argTable,
	)
	if err != nil {
		common.ZapError("rabbitmq exchange declare fail", err)
		return
	}

	//备份交换器
	err = ch.ExchangeDeclare(
		"miaosha_demo_exchange_ae",
		"fanout",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		common.ZapError("rabbitmq exchange ae declare fail", err)
		return
	}

	//死信交换器
	err = ch.ExchangeDeclare(
		"miaosha_demo_exchange_dead",
		"fanout",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		common.ZapError("rabbitmq exchange dead declare fail", err)
		return
	}

	//消费者流量控制
	err = ch.Qos(
		1,
		0,
		false,
	)
	if err != nil {
		common.ZapError("rabbitmq qos fail", err)
		return
	}

	//声明队列，并绑定死信交换器
	myQueueArgs := amqp.Table{
		"x-dead-letter-exchange" : "miaosha_demo_exchange_dead",
		"x-dead-letter-routing-key" : "miaosha_demo",
	}
	q, err := ch.QueueDeclare(
		"miaosha_demo_queue",
		true,
		false,
		false,
		false,
		myQueueArgs,
	)
	if err != nil {
		common.ZapError("rabbitmq queue dechalre fail", err)
		return
	}

	err = ch.QueueBind(
		q.Name,
		"aaa.*.ccc",
		"miaosha_demo_exchange",
		false,
		nil,
	)
	if err != nil {
		common.ZapError("rabbitmq queue bind fail", err)
		return
	}

	//备份队列
	qAe, err := ch.QueueDeclare(
		"miaosha_demo_queue_ae",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		common.ZapError("rabbitmq queue ae declare fail", err)
		return
	}

	err = ch.QueueBind(
		qAe.Name,
		"",
		"miaosha_demo_exchange_ae",
		false,
		nil,
	)
	if err != nil {
		common.ZapError("rabbitmq queue ae bind fail", err)
		return
	}

	//死信队列
	qDead, err := ch.QueueDeclare(
		"miaosha_demo_queue_dead",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		common.ZapError("rabbitmq queue dead declare fail", err)
		return
	}

	err = ch.QueueBind(
		qDead.Name,
		"",
		"miaosha_demo_exchange_dead",
		false,
		nil,
	)
	if err != nil {
		common.ZapError("rabbitmq queue dead bind fail", err)
		return
	}

	//正常消费
	msgs, err := ch.Consume(
		q.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		common.ZapError("rabbitmq register consumer fail", err)
		return
	}

	go func() {
		for d := range msgs {
			orderRepository := repositories.NewOrderRepository(mysqlPoolProduct)
			orderService := services.NewOrderService(orderRepository)

			uidPidSlice := strings.Split(string(d.Body), "_")
			pid, err := strconv.Atoi(uidPidSlice[0])
			if err != nil {
				common.ZapError("rabbitmq get pid fail", err)
			}

			uid , err := strconv.Atoi(uidPidSlice[1])
			if err != nil {
				common.ZapError("rabbitmq get uid fail", err)
			}

			order := &datamodels.Order{}
			order.Uid = uint32(uid)
			order.Pid = uint64(pid)
			order.State = datamodels.OrderWait
			order.CreateAt = time.Now().Unix()

			err = orderService.InsertIgnoreOrder(order)
			if err != nil {
				common.ZapError("rabbitmq add order fail", err)
			}

			//消费后确认
			d.Ack(false)
		}
	}()


	/*
	//备份队列消费
	msgsAe, err := ch.Consume(
		qAe.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	common.FailOnError(err, "Failed to register a consumer2")
	go func() {
		for d2 := range msgsAe {
			log.Printf("ae: [x] %v", d2)
			d2.Ack(false)
		}
	}()

	//死信队列消费
	msgsDead, err := ch.Consume(
		qDead.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	common.FailOnError(err, "Failed to register a consumer3")
	go func() {
		for d3 := range msgsDead {
			log.Printf("dead: [x] %v", d3)
			d3.Ack(false)
		}
	}()
	*/

	log.Printf(" [*] Waiting for logs. To exit press CTRL+C")
	forever := make(chan bool)
	<-forever

}

