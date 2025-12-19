# Tex

TinyExchange(Tex) 是一个go语言编写的交易后流程(Post-Trade)服务。

它通过消费[tte](https://github.com/Tsumida/tte)产生的消息，转储到mysql和redis中, 同时提供查询能力。

# 运行
下载源代码
```
cd ~
git clone https://github.com/Tsumida/tte
git clone https://github.com/Tsumida/tex
```
构建镜像
```
cd ./tte && make build-img
cd ../tex && make build-img
```
拉起镜像并执行测试
```
make test
```

# 下单
构建tte client
```
cd ./tte && cargo build --workspace tte-client
./mvp_client
```
启动client后通过命令行输入交易指令: 
```
OMS:Bid,1001,CLI_1001_00201,LIMIT,BTC_USDT,80050.00,1.0,GTK,1,false    
OMS:Ask,1002,CLI_1002_00201,LIMIT,BTC_USDT,79990.00,0.5,GTK,1,false  
OMS:Bid,1001,CLI_1001_00202,LIMIT,BTC_USDT,80060.00,2.0,GTK,1,false    
OMS:Ask,1002,CLI_1002_00202,LIMIT,BTC_USDT,80060.00,1.5,GTK,1,false 
OMS:Cancel,Bid,1001,CLI_1001_00201,BTC_USDT
OMS:Cancel,Bid,1001,CLI_1001_00202,BTC_USDT
```
