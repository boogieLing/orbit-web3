# UsedIt API 合同 v1

前缀 /api/v1。JSON。写请求需要 X-UsedIt-Request: 1，浏览器同源 Cookie；错误 {error:{code,message},request_id}。会话由服务端生成，客户端不传 user_id。时间为 ISO8601 UTC。

## 类型

App: {id,name,category,summary,url,chain_id,contracts:string[],tags:string[],accent,mark,source_url}。
User: {id,display_name,wallet:string|null,preferences:string[]}。
Post: {id,kind:"review"|"reference",app_id,title,body,sentiment:"positive"|"neutral"|"negative",tags:string[],author:{name,wallet:string|null},proof_status:"none"|"pending"|"verified"|"unavailable"|"rejected",proof_message,tx_hash,proof_checked_at,created_at,updated_at,revision,helpful_count,bookmarked,helpful,owned,source_url?,reason?}。
Collection: {id,title,description,instructions,layout:"editorial"|"grid",theme:"ink"|"sage"|"sand",app_ids:string[],post_ids:string[],version,html,created_at,updated_at}。

## 接口

GET /session -> {user,capabilities:{wallet:true,proof:true,freeform:boolean},stats:{reviews,verified,apps}}，无会话时初始化游客。
POST /auth/challenge {address} -> {challenge_id,message}；钱包 personal_sign 签 message。
POST /auth/verify {challenge_id,signature} -> {user} 并替换 Cookie。
POST /auth/logout {} -> {ok:true}。
PATCH /profile {display_name} -> {user}。
GET /apps -> {items:App[]}。
GET /feed?tab=for-you|latest|verified|saved&category=all|identity|exchange|collectibles&q=&cursor= -> {items:Post[],next_cursor:string|null}。
GET /posts/{id} -> {post:Post}。
POST /posts {app_id,title,body,sentiment,tags,tx_hash?}，Idempotency-Key UUID，需钱包身份 -> 201 {post:Post}。创建时凭据为none，若有tx_hash前端另调verify。
PATCH /posts/{id} {title,body,sentiment,tags,revision} -> {post:Post}，版本冲突409，仅本人。
DELETE /posts/{id} -> {ok:true}，软删除，仅本人。
POST /posts/{id}/verify {tx_hash} -> {post:Post}，仅本人。有待确认、拒绝、上游不可用状态都明确返回；不自动重试。
PUT /posts/{id}/bookmark {active:boolean} -> {active}。
PUT /posts/{id}/helpful {active:boolean} -> {active}。
POST /feedback {post_id,kind:"open"|"hide"|"impression",event_id:string,active?:boolean} -> {ok:true}。
PUT /preferences {tags:string[]} -> {user}。
GET /me/posts -> {items:Post[]}。
GET /collections -> {items:Collection[]}。
POST /collections {title,description,instructions,layout,theme,app_ids,post_ids} -> {collection:Collection}。结构化内容编译出单文件HTML；最多各12项。
GET /collections/{id} -> {collection,versions:[{version,created_at}]}，仅本人。
PATCH /collections/{id} 与创建相同另加 version -> {collection}，保存新版本，冲突409。
GET /collections/{id}/versions/{number} -> {collection}，仅本人。
GET /collections/{id}/export -> HTML附件，仅本人。
POST /collections/{id}/compose {instruction} -> 外部创作服务处理形成新版本，未配置时503并明确提示。

## 界面约束

中文品牌“用过再说”，英文 UsedIt。不使用用户禁止措辞。官方资料必须标“资料笔记”，不能装作用户使用体验；无真实用户评价时显示真实空态。上方精选可以展示带来源的资料笔记。发布流程钱包验证后允许，取消签名不能丢草稿。所有按钮必须实际动作；来源、外站、钱包没有时给出可理解提示。
创建精选页需要真实HTML预览、复制/下载、版本；预览sandbox只allow-scripts，无同源、无外部请求。自由需求未配置必须说明，结构化创建仍可用。
推荐理由、凭据标识使用准确文案，不用人物/安全认证的语气。余额、收益不进入推荐逻辑。
