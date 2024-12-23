package ratelimit

import "github.com/redis/go-redis/v9"

// the first key is the key which to store the ratelimit
// the first argument is burst, which is the amount of tokens in the buckets
// the second argument is rate, which is number of tokens which are returned to the bucket
// the third argument is period, which is the time period for accounting
// the fourth argument is cost, the price of the request
var LuaAllowN = redis.NewScript(luaAllowNScript)

// slightly modified version of the scripts from here
// https://github.com/rwz/redis-gcra/blob/master/vendor/perform_gcra_ratelimit.lua

var luaAllowNScript = `
-- this script has side-effects, so it requires replicate commands mode
redis.replicate_commands()

local rate_limit_key = KEYS[1]
local banned_rate_limit_key = KEYS[2]
local stream_key = KEYS[3]
local burst = ARGV[1]
local rate = ARGV[2]
local period = ARGV[3]
local cost = tonumber(ARGV[4])

local emission_interval = period / rate
local increment = emission_interval * cost
local burst_offset = emission_interval * burst

-- redis returns time as an array containing two integers: seconds of the epoch
-- time (10 digits) and microseconds (6 digits). for convenience we need to convert them to a floating point number.
-- the resulting number is 16 digits,
-- bordering on the limits of a 64-bit double-precision floating point number.
local now = redis.call("TIME")


-- this script used to adjust the epoch to be relative to Jan 1, 2017 00:00:00 GMT to avoid floating
-- point problems. that approach is good until "now" is 2,483,228,799 (Wed, 09 Sep 2048 01:46:39 GMT), when the adjusted value is 16 digits.
-- local jan_1_2017 = 1483228800
-- now = (now[1] - jan_1_2017) + (now[2] / 1000000)


-- instead of doing that, i reduce to millisecond precision. this makes the number smaller. i don't care about floating point errors, this is imprecise anyway beacuse it's async
now = (now[1]) + (math.floor(now[2] / 1000) / 1000)

local tat = redis.call("GET", rate_limit_key)

if not tat then
  tat = now
else
  tat = tonumber(tat)
end

tat = math.max(tat, now)

local new_tat = tat + increment
local allow_at = new_tat - burst_offset

local diff = now - allow_at
local remaining = diff / emission_interval

-- here the rate limit is hit
if remaining < 0 then
  local reset_after = tat - now
  local retry_after = diff * -1
	-- this is disabled for now beacuse double banning is fine i guess -- we get the banned key, and see if it exists. this is so we don't double ban people.
--local bannedUntil = redis.call("GET", banned_rate_limit_key)
--if not bannedUntil then
		-- ban the user
	  redis.call("XADD", stream_key, "*", "user", rate_limit_key, "action", "ban", "until", tostring(math.ceil(tat)))
	  -- set the user banned until they reach the retry_again time. this prevents us from submitting too many stream items
	--  redis.call("SET", banned_rate_limit_key, math.ceil(reset_after), "EX", math.ceil(reset_after))
--else
--  -- at this point, the user is already banned, so there is nothing we need to do! yay
--end
  return {
    0, -- allowed
    0, -- remaining
    tostring(retry_after),
    tostring(reset_after),
  }
end

local reset_after = new_tat - now
if reset_after > 0 then
  redis.call("SET", rate_limit_key, new_tat, "EX", math.ceil(reset_after))
end
local retry_after = -1
return {cost, remaining, tostring(retry_after), tostring(reset_after)}
`
