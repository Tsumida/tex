-- 数据库规范

-- DB要求: 字符集 utf8mb4, 排序规则 utf8mb4_0900_ai_ci

-- 表要求: 字段名全部小写, 下划线分隔

-- 字段要求
-- 时间戳: UTC时间戳, 单位可以是sec, milli, micro。 默认为micro
-- 时间戳字段: create_time, update_time, delete_time 均为 unsigned bigint, 内存对应 uint64. 必须手动更新，不依赖数据库自动更新
-- 自增ID: 所有表必须用 unsigned bigint 作为自增ID的类型, 内存对应 uint64. 每个表必须有自己的自增ID
-- 并发控制: 除了日志表外, 所有表必须有 version 字段, 类型为 unsigned bigint, 内存对应 uint64. 每次更新时 version + 1
-- 金额字段: 涉及金额的字段必须统一使用Decimal(65, 18)
-- 事务时间: 即撮合完成等业务时间戳
-- 所有字段必须有default, not null和注释. Text字段除外
-- 交易对: 统一使用 VARCHAR(16), 格式为BTC_USDT. 用分隔符方便处理

-- 设置utf8mb4字符集, 避免乱码
SET NAMES utf8mb4 COLLATE utf8mb4_0900_ai_ci;

-- 创建数据库
CREATE DATABASE IF NOT EXISTS `tex` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
USE `tex`;


-- match_result表, 撮合成交结果汇总表, 包含交易对
CREATE TABLE IF NOT EXISTS `match_result` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '自增ID',
    `seq_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '撮合器sequencer自增ID',
    `prev_seq_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '上次撮合ID, 用于幂等',
    `trading_pair` VARCHAR(16) NOT NULL DEFAULT '' COMMENT '交易对,BTC_USDT格式',
    `biz_action` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '业务动作, 对应BizAction枚举',
    `payload` TEXT COMMENT '撮合结果的详细信息',
    `tx_time` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '撮合时间',
    `create_time` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '创建时间',
    `update_time` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '更新时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `udx_seq_id` (`seq_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='撮合输出日志表';

-- fill表, 用于存储撮合结果, 包含买卖双方订单号、成交价格、成交量等关键信息。
CREATE TABLE IF NOT EXISTS `fill` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '自增ID',
    `match_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '成交ID',
    `prev_match_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '上次成交ID, 用于幂等',
    `seq_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '撮合器sequencer自增ID, 用于和match_result关联',
    `taker_order_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '吃单ID',
    `maker_order_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '挂单ID',
    `taker_account_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '吃单用户ID',
    `maker_account_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '挂单用户ID',
    `price` DECIMAL(65, 18) NOT NULL DEFAULT 0.000000000000000000 COMMENT '成交价格',
    `qty` DECIMAL(65, 18) NOT NULL DEFAULT 0.000000000000000000 COMMENT '成交量',
    `direction` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '方向,Bid,Ask',
    `tx_time` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '成交时间',
    `create_time` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '创建时间',
    `update_time` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '更新时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `udx_match_id` (`match_id`),
    KEY `idx_seq_id` (`seq_id`),
    KEY `idx_taker_order_id` (`taker_order_id`),
    KEY `idx_maker_order_id` (`maker_order_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='成交明细日志表';


-- order表, 用于查询用户所有订单，包含订单号、已成交、目标成交量等关键信息。
CREATE TABLE IF NOT EXISTS `order` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '自增ID',
    `order_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '订单ID',
    `account_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '用户ID',
    `trading_pair` VARCHAR(16) NOT NULL DEFAULT '' COMMENT '交易对',
    `order_type` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '订单类型, 对应OrderType枚举',
    `direction` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '方向,Bid,Ask',
    `target_qty` DECIMAL(65, 18) NOT NULL DEFAULT 0.000000000000000000 COMMENT '目标成交量',
    `filled_qty` DECIMAL(65, 18) NOT NULL DEFAULT 0.000000000000000000 COMMENT '已成交量',
    `price` DECIMAL(65, 18) NOT NULL DEFAULT 0.000000000000000000 COMMENT '委托价格',
    `order_state` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '订单状态, 对应OrderState枚举',
    `version` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '版本号, 用于并发控制',
    `create_time` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '创建时间',
    `update_time` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '更新时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `udx_order_id` (`order_id`),
    KEY `idx_account_id` (`account_id`),
    KEY `idx_trading_pair` (`trading_pair`),
    KEY `idx_order_state` (`order_state`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='订单汇总表';


-- order_detail表, 用于存储订单的详细信息(PB or JSON)。
CREATE TABLE IF NOT EXISTS `order_detail` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '自增ID',
    `order_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '订单ID',
    `version` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '版本号, 用于并发控制',
    `detail` TEXT COMMENT '订单详细信息',
    `create_time` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '创建时间',
    `update_time` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '更新时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `udx_order_id` (`order_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='订单详细信息表';

-- balance表，储存账户最新资产. 
CREATE TABLE IF NOT EXISTS `balance` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '自增ID',
    `account_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '用户ID',
    `currency` VARCHAR(8) NOT NULL DEFAULT '' COMMENT '币种',
    `deposit` DECIMAL(65, 18) NOT NULL DEFAULT 0.000000000000000000 COMMENT '可用资产',
    `frozen` DECIMAL(65, 18) NOT NULL DEFAULT 0.000000000000000000 COMMENT '冻结资产',
    `balance` DECIMAL(65, 18) NOT NULL DEFAULT 0.000000000000000000 COMMENT '总资产',
    `version` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '版本号, 用于并发控制',
    `create_time` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '创建时间',
    `update_time` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '更新时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `udx_account_currency` (`account_id`, `currency`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='账户资产表';
