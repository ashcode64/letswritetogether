package script

import (
	"context"

	"github.com/redis/go-redis/v9"
)

// SubmitLua inserts one exact text per round.
// Two devices that send the identical text share one word id.
// hello and heLLO are different fields.
const SubmitLua = `
local text = ARGV[1]
local ttl = tonumber(ARGV[2])
local prev = redis.call('GET', KEYS[2])
if prev then
  local old = redis.call('HGET', KEYS[3], prev)
  if old == text then
    return {prev, '0', 'SAME'}
  end
  return {prev, '0', 'ALREADY'}
end
local id = redis.call('HGET', KEYS[1], text)
if id then
  redis.call('SET', KEYS[2], id, 'EX', ttl)
  redis.call('EXPIRE', KEYS[1], ttl)
  return {id, '0', 'JOIN'}
end
id = tostring(redis.call('INCR', KEYS[5]))
redis.call('HSET', KEYS[1], text, id)
redis.call('HSET', KEYS[3], id, text)
redis.call('RPUSH', KEYS[4], id)
redis.call('SET', KEYS[2], id, 'EX', ttl)
redis.call('EXPIRE', KEYS[1], ttl)
redis.call('EXPIRE', KEYS[3], ttl)
redis.call('EXPIRE', KEYS[4], ttl)
redis.call('EXPIRE', KEYS[5], ttl)
return {id, '1', 'NEW'}
`

// VoteLua is the atomic last-write-wins vote.
// A seq older than the one stored is ignored.
const VoteLua = `
local word = ARGV[1]
local seq = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
local curSeq = redis.call('HGET', KEYS[1], 'seq')
local curWord = redis.call('HGET', KEYS[1], 'word')
if curSeq and tonumber(curSeq) >= seq then
  return {'STALE', curWord or '', curSeq}
end
if word == '' then
  if curWord and curWord ~= false and curWord ~= '' then
    redis.call('HINCRBY', KEYS[2], curWord, -1)
  end
  redis.call('HSET', KEYS[1], 'word', '', 'seq', tostring(seq))
  redis.call('EXPIRE', KEYS[1], ttl)
  redis.call('EXPIRE', KEYS[2], ttl)
  return {'OK', '', tostring(seq)}
end
if redis.call('HEXISTS', KEYS[4], word) == 0 then
  return {'MISSING', '', ''}
end
if (not curWord) or curWord == false or curWord == '' then
  redis.call('HSET', KEYS[1], 'word', word, 'seq', tostring(seq))
  redis.call('HINCRBY', KEYS[2], word, 1)
elseif curWord == word then
  redis.call('HSET', KEYS[1], 'seq', tostring(seq))
else
  redis.call('HINCRBY', KEYS[2], curWord, -1)
  redis.call('HINCRBY', KEYS[2], word, 1)
  redis.call('HSET', KEYS[1], 'word', word, 'seq', tostring(seq))
end
redis.call('EXPIRE', KEYS[1], ttl)
redis.call('EXPIRE', KEYS[2], ttl)
return {'OK', word, tostring(seq)}
`

var (
	submitScript = redis.NewScript(SubmitLua)
	voteScript   = redis.NewScript(VoteLua)
)

// RunSubmit returns word id, created, and a status of NEW, JOIN, SAME, or ALREADY.
func RunSubmit(ctx context.Context, rdb redis.Scripter, keys []string, text string, ttl int) (id string, created bool, status string, err error) {
	v, err := submitScript.Run(ctx, rdb, keys, text, ttl).Slice()
	if err != nil {
		return "", false, "", err
	}
	id, _ = v[0].(string)
	flag, _ := v[1].(string)
	status, _ = v[2].(string)
	return id, flag == "1", status, nil
}

// RunVote returns a status of OK, STALE, or MISSING.
func RunVote(ctx context.Context, rdb redis.Scripter, keys []string, wordID string, seq, ttl int) (status, word string, err error) {
	v, err := voteScript.Run(ctx, rdb, keys, wordID, seq, ttl).Slice()
	if err != nil {
		return "", "", err
	}
	status, _ = v[0].(string)
	word, _ = v[1].(string)
	return status, word, nil
}
