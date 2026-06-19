schema "public" {}

table "candles" {
  schema = schema.public
  column "id" {
    null = false
    type = bigint
    identity {
      generated = BY_DEFAULT
    }
  }
  column "symbol" {
    null  = false
    type  = text
  }
  column "open_time" {
    null = false
    type = timestamptz
  }
  column "open" {
    null = false
    type = bigint
  }
  column "high" {
    null = false
    type = bigint
  }
  column "low" {
    null = false
    type = bigint
  }
  column "close" {
    null = false
    type = bigint
  }
  column "volume" {
    null = false
    type = bigint
  }
  column "inside_bid" {
    null = false
    type = bigint
  }
  column "inside_ask" {
    null = false
    type = bigint
  }
  column "source" {
    null    = false
    type    = text
    default = "ticker"
  }
  column "ingested_at" {
    null    = false
    type    = timestamptz
    default = sql("now()")
  }
  primary_key {
    columns = [column.id]
  }
  index "candles_symbol_open_time_key" {
    unique  = true
    columns = [column.symbol, column.open_time]
  }
}

table "orders" {
  schema = schema.public
  column "id" {
    null = false
    type = bigint
    identity {
      generated = BY_DEFAULT
    }
  }
  column "client_uid" {
    null = false
    type = uuid
  }
  column "symbol" {
    null = false
    type = text
  }
  column "side" {
    null = false
    type = text
  }
  column "type" {
    null = false
    type = text
  }
  column "limit_price" {
    null = true
    type = bigint
  }
  column "quantity" {
    null = false
    type = bigint
  }
  column "mode" {
    null = false
    type = text
  }
  column "strategy" {
    null = true
    type = text
  }
  column "status" {
    null = false
    type = text
  }
  column "exchange_ref" {
    null = true
    type = text
  }
  column "reason" {
    null = true
    type = text
  }
  column "created_at" {
    null    = false
    type    = timestamptz
    default = sql("now()")
  }
  primary_key {
    columns = [column.id]
  }
  index "orders_client_uid_key" {
    unique  = true
    columns = [column.client_uid]
  }
}

table "positions" {
  schema = schema.public
  column "id" {
    null = false
    type = bigint
    identity {
      generated = BY_DEFAULT
    }
  }
  column "symbol" {
    null = false
    type = text
  }
  column "qty" {
    null    = false
    type    = bigint
    default = 0
  }
  column "avg_cost" {
    null    = false
    type    = bigint
    default = 0
  }
  column "realized_pnl" {
    null    = false
    type    = bigint
    default = 0
  }
  column "updated_at" {
    null    = false
    type    = timestamptz
    default = sql("now()")
  }
  primary_key {
    columns = [column.id]
  }
  index "positions_symbol_key" {
    unique  = true
    columns = [column.symbol]
  }
}

table "risk_state" {
  schema = schema.public
  column "id" {
    null = false
    type = bigint
    identity {
      generated = BY_DEFAULT
    }
  }
  column "day" {
    null = false
    type = date
  }
  column "halted" {
    null    = false
    type    = boolean
    default = false
  }
  column "halt_reason" {
    null = true
    type = text
  }
  column "spent_today" {
    null    = false
    type    = bigint
    default = 0
  }
  column "loss_today" {
    null    = false
    type    = bigint
    default = 0
  }
  column "updated_at" {
    null    = false
    type    = timestamptz
    default = sql("now()")
  }
  primary_key {
    columns = [column.id]
  }
  index "risk_state_day_key" {
    unique  = true
    columns = [column.day]
  }
}

table "session_tracks" {
  schema = schema.public
  column "id" {
    null = false
    type = bigint
    identity {
      generated = BY_DEFAULT
    }
  }
  column "session_event" {
    null = false
    type = integer
  }
  column "balance" {
    null = false
    type = bigint
  }
  column "event_at" {
    null    = false
    type    = timestamptz
    default = sql("now()")
  }
  primary_key {
    columns = [column.id]
  }
}
