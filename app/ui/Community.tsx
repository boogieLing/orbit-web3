"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ArrowDown, ArrowRight, ArrowUpRight, BookOpen, Bookmark, ChevronRight, Clock3, Compass, FileText, FolderHeart, HeartHandshake, Link2, ListFilter, LoaderCircle, Menu, Pencil, Plus, Search, Settings2, SlidersHorizontal, Wallet, X } from "lucide-react";
import { api, dateLabel, errorText, mutation, shortAddress, signIn } from "./api";
import type { AppInfo, Collection, Feed, Post, Session, Tab, User } from "./api";
import { AppMark, Busy, categories, categoryName, Empty, ExternalLink, Logo, Modal, Notice, SourceTag, Toast } from "./shared";
import PostCard, { PostDetail } from "./PostCard";
import PostEditor, { blankDraft } from "./PostEditor";
import type { PostDraft } from "./PostEditor";
import CollectionEditor from "./CollectionEditor";
import Settings from "./Settings";

type View = "discover" | "apps" | "collections" | "posts";
type Feedback = { text: string; action?: { label: string; run: () => void } };
const tabs: { id: Tab; title: string; Icon: typeof Compass }[] = [{ id: "for-you", title: "为你推荐", Icon: Compass }, { id: "latest", title: "最新分享", Icon: Clock3 }, { id: "verified", title: "有交互凭据", Icon: Link2 }, { id: "saved", title: "我的收藏", Icon: Bookmark }];

// 协调真实社区会话、内容流、用户反馈和所有编辑入口。
export default function Community() {
  const [session, setSession] = useState<Session | null>(null);
  const [apps, setApps] = useState<AppInfo[]>([]);
  const [feed, setFeed] = useState<Feed>({ items: [], next_cursor: null });
  const [collections, setCollections] = useState<Collection[]>([]);
  const [myPosts, setMyPosts] = useState<Post[]>([]);
  const [view, setView] = useState<View>("discover");
  const [tab, setTab] = useState<Tab>("for-you");
  const [category, setCategory] = useState("all");
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [bootError, setBootError] = useState("");
  const [loadedKey, setLoadedKey] = useState("");
  const [loadMore, setLoadMore] = useState(false);
  const [feedError, setFeedError] = useState("");
  const [reload, setReload] = useState(0);
  const [toast, setToast] = useState<Feedback | null>(null);
  const [detail, setDetail] = useState<Post | null>(null);
  const [opening, setOpening] = useState<string | null>(null);
  const [postEditor, setPostEditor] = useState(false);
  const [editingPost, setEditingPost] = useState<Post | null>(null);
  const [draft, setDraft] = useState<PostDraft>(blankDraft);
  const [editDraft, setEditDraft] = useState<PostDraft>(blankDraft);
  const [collectionEditor, setCollectionEditor] = useState(false);
  const [editingCollection, setEditingCollection] = useState<Collection | null>(null);
  const [collectionPosts, setCollectionPosts] = useState<Post[]>([]);
  const [settings, setSettings] = useState(false);
  const [help, setHelp] = useState(false);
  const [mobileMenu, setMobileMenu] = useState(false);
  const [walletBusy, setWalletBusy] = useState(false);
  const [pending, setPending] = useState<string | null>(null);
  const actionLock = useRef(false);
  const loadGeneration = useRef(0);
  const mounted = useRef(true);

  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  useEffect(() => {
    if (!toast || toast.action) return;
    const timeout = window.setTimeout(() => setToast(null), 6500);
    return () => window.clearTimeout(timeout);
  }, [toast]);

  useEffect(() => {
    const controller = new AbortController();
    api<Session>("/session", { signal: controller.signal }).then(async state => {
      // 建立会话后再发依赖 Cookie 的请求，避免首次访问出现多个游客身份。
      const result = await api<{ items: AppInfo[] }>("/apps", { signal: controller.signal });
      if (!controller.signal.aborted) { setSession(state); setApps(result.items || []); setBootError(""); }
    }).catch(reason => { if (!controller.signal.aborted) setBootError(errorText(reason)); });
    return () => controller.abort();
  }, [reload]);

  const userID = session?.user.id;
  const requestKey = [userID, view, tab, category, query, reload].join("|");
  const booting = !session && !bootError;
  const loading = view !== "apps" && loadedKey !== requestKey;
  useEffect(() => {
    if (!userID) return;
    const controller = new AbortController();
    const generation = ++loadGeneration.current;
    const params = new URLSearchParams({ tab, category, q: query });
    const path = view === "collections" ? "/collections" : view === "posts" ? "/me/posts" : `/feed?${params}`;
    if (view === "apps") return () => controller.abort();
    api<{ items: (Post | Collection)[]; next_cursor?: string | null }>(path, { signal: controller.signal }).then(result => {
      if (controller.signal.aborted || generation !== loadGeneration.current) return;
      setFeedError("");
      if (view === "collections") setCollections(result.items as Collection[]);
      else if (view === "posts") setMyPosts(result.items as Post[]);
      else setFeed({ items: result.items as Post[], next_cursor: result.next_cursor || null });
    }).catch(reason => { if (!controller.signal.aborted) setFeedError(errorText(reason)); }).finally(() => { if (!controller.signal.aborted && generation === loadGeneration.current) setLoadedKey(requestKey); });
    return () => controller.abort();
  }, [userID, view, tab, category, query, reload, requestKey]);

  // 更新业务身份或偏好，并重新读取该身份可访问的内容。
  const updateUser = (user: User) => { setSession(value => value ? { ...value, user } : value); setReload(value => value + 1); };
  // 显示一条操作回执，必要时附上实际可执行的撤销动作。
  const notify = (text: string, action?: Feedback["action"]) => setToast({ text, action });
  // 重新读取服务端统计，不让附属请求覆盖已完成的内容操作。
  const refreshStats = async () => { try { const result = await api<Session>("/session"); setSession(result); } catch { /* 内容提交结果优先，统计信息可在下次访问更新。 */ } };
  // 将服务端内容变更同步到列表、详情和精选候选内容。
  const patchPost = useCallback((post: Post) => {
    // 在已有列表中替换同一内容，保持其他条目的原有顺序。
    const replace = (items: Post[]) => items.map(value => value.id === post.id ? post : value);
    setFeed(value => ({ ...value, items: replace(value.items) })); setMyPosts(replace); setCollectionPosts(replace);
    setDetail(value => value?.id === post.id ? post : value);
  }, []);
  // 完成钱包签名登录，成功后刷新业务身份与内容。
  const connectWallet = async () => {
    setWalletBusy(true);
    try { const user = await signIn(); updateUser(user); notify("钱包身份已连接，可以发布体验了。"); }
    finally { setWalletBusy(false); }
  };
  // 退出钱包会话并重新初始化可继续浏览的游客身份。
  const logout = async () => { await api("/auth/logout", mutation("POST", {})); setDetail(null); setReload(value => value + 1); notify("已退出钱包身份。"); };
  // 切换主视图，并收起手机导航及上一个视图的错误状态。
  const navigate = (next: View) => { setView(next); setMobileMenu(false); setFeedError(""); };
  // 读取最新详情后打开文章，并记录实际打开反馈。
  const openPost = async (post: Post) => {
    setOpening(post.id);
    try {
      const result = await api<{ post: Post }>(`/posts/${post.id}`); setDetail(result.post); patchPost(result.post);
      // 打开反馈为附属请求，其失败不阻止已读取的正文展示。
      void api("/feedback", mutation("POST", { post_id: post.id, kind: "open", event_id: crypto.randomUUID() })).catch(() => {});
    } catch (reason) { notify(errorText(reason)); } finally { setOpening(null); }
  };
  // 等待服务端确认后更新收藏或有用状态，避免重复提交。
  const togglePost = async (post: Post, kind: "bookmark" | "helpful") => {
    if (actionLock.current) return;
    actionLock.current = true; setPending(post.id);
    try {
      const old = kind === "bookmark" ? post.bookmarked : post.helpful;
      const result = await api<{ active: boolean }>(`/posts/${post.id}/${kind}`, mutation("PUT", { active: !old }));
      // 以回执中的最终状态计算计数差，重复状态不会再累加。
      const changed = kind === "bookmark" ? { ...post, bookmarked: result.active } : { ...post, helpful: result.active, helpful_count: Math.max(0, post.helpful_count + (result.active === old ? 0 : result.active ? 1 : -1)) };
      patchPost(changed);
      if (kind === "bookmark") {
        notify(result.active ? "已收藏，留到下次慢慢看。" : "已从收藏中移除。");
        if (!result.active && tab === "saved") setFeed(value => ({ ...value, items: value.items.filter(item => item.id !== post.id) }));
      }
    } catch (reason) { notify(errorText(reason)); } finally { actionLock.current = false; setPending(null); }
  };
  // 保存不感兴趣反馈并移除内容，同时提供服务端撤销操作。
  const hidePost = async (post: Post) => {
    if (actionLock.current) return;
    actionLock.current = true; setPending(post.id);
    try {
      await api("/feedback", mutation("POST", { post_id: post.id, kind: "hide", event_id: crypto.randomUUID(), active: true }));
      // 隐藏成功后才移出列表，撤销时另发明确的取消反馈并重新读取。
      setFeed(value => ({ ...value, items: value.items.filter(item => item.id !== post.id) }));
      notify("这条内容已隐藏，推荐会参考你的反馈。", { label: "撤销", run: () => { void api("/feedback", mutation("POST", { post_id: post.id, kind: "hide", event_id: crypto.randomUUID(), active: false })).then(() => { setReload(value => value + 1); notify("已恢复这条内容。"); }).catch(reason => notify(errorText(reason))); } });
    } catch (reason) { notify(errorText(reason)); } finally { actionLock.current = false; setPending(null); }
  };
  // 按游标继续加载内容，并丢弃视图切换前返回的旧请求。
  const nextPage = async () => {
    if (!feed.next_cursor || loadMore) return;
    const generation = loadGeneration.current;
    setLoadMore(true);
    try {
      const params = new URLSearchParams({ tab, category, q: query, cursor: feed.next_cursor });
      const result = await api<Feed>(`/feed?${params}`);
      // 仅将同一视图轮次的结果合并，并按内容标识去掉分页重叠项。
      if (generation === loadGeneration.current) setFeed(value => ({ items: [...new Map([...value.items, ...result.items].map(post => [post.id, post])).values()], next_cursor: result.next_cursor }));
    } catch (reason) { if (generation === loadGeneration.current) notify(errorText(reason)); } finally { if (mounted.current) setLoadMore(false); }
  };
  // 打开新体验表单，可预选应用且保留未发布的草稿。
  const newPost = (appID?: string) => { setEditingPost(null); if (appID) setDraft(value => ({ ...value, app_id: appID })); setPostEditor(true); setMobileMenu(false); };
  // 从作者当前内容建立编辑草稿，随后按版本提交修改。
  const editPost = (post: Post) => { setEditingPost(post); setEditDraft({ app_id: post.app_id, title: post.title, body: post.body, sentiment: post.sentiment, tags: post.tags.join("，"), tx_hash: post.tx_hash || "" }); setDetail(null); setPostEditor(true); };
  // 处理真实发布结果，更新相关视图并重置已成功提交的草稿。
  const savePost = (post: Post, note: string) => { patchPost(post); setPostEditor(false); if (!editingPost) setDraft(blankDraft()); setEditingPost(null); setDetail(post); notify(note); setReload(value => value + 1); void refreshStats(); };
  // 提交删除并同步移除列表与详情，统计信息随后刷新。
  const deletePost = async (post: Post) => {
    await api(`/posts/${post.id}`, { method: "DELETE" });
    setFeed(value => ({ ...value, items: value.items.filter(item => item.id !== post.id) })); setMyPosts(value => value.filter(item => item.id !== post.id)); setDetail(null); notify("这条体验已删除。"); void refreshStats();
  };
  // 汇总真实收藏和已读内容，读取缺失引用后打开精选编辑器。
  const openCollection = async (collection: Collection | null) => {
    setOpening(collection?.id || "new-collection");
    try {
      const result = await api<Feed>("/feed?tab=saved&category=all&q=");
      const selected = collection?.post_ids.filter(id => ![...result.items, ...feed.items, ...myPosts].some(post => post.id === id)) || [];
      // 单独补齐历史精选的引用；某篇已无法访问时仍保留其余候选内容。
      const extra = await Promise.allSettled(selected.map(id => api<{ post: Post }>(`/posts/${id}`)));
      const resolved = extra.flatMap(item => item.status === "fulfilled" ? [item.value.post] : []);
      setCollectionPosts([...new Map([...result.items, ...feed.items, ...myPosts, ...resolved].map(post => [post.id, post])).values()]);
      setEditingCollection(collection); setCollectionEditor(true);
    } catch (reason) { notify(errorText(reason)); } finally { setOpening(null); }
  };
  // 将保存后的精选版本写回个人列表。
  const saveCollection = (collection: Collection) => { setCollections(value => [collection, ...value.filter(item => item.id !== collection.id)]); };
  const appMap = new Map(apps.map(app => [app.id, app]));
  const feature = !query && category === "all" && tab === "for-you" ? feed.items.find(post => post.kind === "reference") : undefined;
  const activePosts = view === "posts" ? myPosts : feed.items;
  const filteredApps = apps.filter(app => (category === "all" || app.category === category) && (!query || `${app.name} ${app.summary} ${app.tags.join(" ")}`.toLowerCase().includes(query.toLowerCase())));
  // 提交搜索关键词，并进入能够展示搜索结果的内容视图。
  const searchSubmit = (event: React.FormEvent) => { event.preventDefault(); setQuery(search.trim()); if (!["discover", "apps"].includes(view)) setView("discover"); };

  return <div className="site-shell">
    <header className="site-header"><div className="header-inner"><button className="brand-button" aria-label="用过再说首页" onClick={() => { navigate("discover"); setTab("for-you"); setCategory("all"); setQuery(""); setSearch(""); }}><Logo /></button><nav className="desktop-nav" aria-label="主导航"><button className={view === "discover" ? "active" : ""} onClick={() => navigate("discover")}>发现</button><button className={view === "apps" ? "active" : ""} onClick={() => navigate("apps")}>应用目录</button><button className={view === "collections" ? "active" : ""} onClick={() => navigate("collections")}>我的精选</button></nav><form className="header-search" onSubmit={searchSubmit}><Search size={17} /><input aria-label="搜索应用、体验和标签" placeholder="搜索应用、体验…" value={search} onChange={event => setSearch(event.target.value)} /><button type="submit" aria-label="搜索"><ArrowRight size={15} /></button></form><div className="header-actions">{session?.user.wallet ? <button className="account-button" disabled={booting} onClick={() => setSettings(true)}><span>{session.user.display_name.slice(0, 1)}</span><span>{shortAddress(session.user.wallet)}</span></button> : <button className="wallet-button" disabled={walletBusy || !session} onClick={() => void connectWallet().catch(reason => notify(errorText(reason)))}>{walletBusy ? <LoaderCircle size={17} className="spin" /> : <Wallet size={17} />}<span>{walletBusy ? "等待钱包" : "连接钱包"}</span></button>}<button className="button primary write-button" disabled={!session} onClick={() => newPost()}><Pencil size={15} />写体验</button><button className="icon-button mobile-menu-button" aria-expanded={mobileMenu} aria-label="展开导航" onClick={() => setMobileMenu(value => !value)}>{mobileMenu ? <X size={21} /> : <Menu size={21} />}</button></div></div>{mobileMenu && <nav className="mobile-nav"><button onClick={() => navigate("discover")}><Compass size={16} />发现</button><button onClick={() => navigate("apps")}><BookOpen size={16} />应用目录</button><button onClick={() => navigate("collections")}><FolderHeart size={16} />我的精选</button><button disabled={!session} onClick={() => { setSettings(true); setMobileMenu(false); }}><Settings2 size={16} />阅读偏好</button></nav>}</header>

    {booting && !session ? <main className="boot-state"><Busy label="正在打开社区" /><p>好内容，值得多等一小会。</p></main> : bootError ? <main className="boot-state"><Notice error>{bootError}</Notice><button className="button primary" onClick={() => { setBootError(""); setReload(value => value + 1); }}>重新连接</button></main> : session && <>
      {view === "discover" && <section className="editorial-hero"><div className="hero-copy"><div className="eyebrow"><span />WEB3, FROM EXPERIENCE.</div><h1>好不好用，<br /><span>用过再说。</span><i className="hero-asterisk"><HeartHandshake size={45} strokeWidth={1.15} /></i></h1><p>发现值得一试的链上应用。<br />读具体的体验，留有用的线索，找到自己的选择。</p><div className="hero-links"><button className="text-link" onClick={() => newPost()}>写下我的体验<ArrowUpRight size={16} /></button><span>真实使用 · 清晰来源 · 自己判断</span></div></div><div className="hero-feature">{feature ? <><div className="feature-caption"><span>从一份资料笔记开始</span><BookOpen size={15} /></div><div className="feature-app"><AppMark app={appMap.get(feature.app_id)} large /><span>{appMap.get(feature.app_id)?.name || "应用笔记"}<small>{categoryName(appMap.get(feature.app_id)?.category || "all")}</small></span><SourceTag post={feature} /></div><button className="feature-title" onClick={() => void openPost(feature)}>{feature.title}<ArrowUpRight size={23} /></button><p>{feature.body}</p><div className="feature-footer"><span>来自官方资料 · 非使用评价</span><button onClick={() => void openPost(feature)} aria-label="阅读精选资料笔记"><ArrowRight size={20} /></button></div></> : <><div className="feature-caption"><span>把选择交还给使用者</span><BookOpen size={15} /></div><h2>少一点抽象介绍，<br />多一点亲身体验。</h2><p>关注具体场景、实际步骤和真实取舍。还没用过，也可以先读资料、收下线索。</p><button className="text-link" onClick={() => { setTab("latest"); setQuery(""); setSearch(""); }}>看看最新分享<ArrowRight size={17} /></button></>}</div></section>}

      {view !== "discover" && <section className="page-heading"><div><div className="eyebrow">{view === "apps" ? "THE APPLICATION INDEX" : view === "collections" ? "CURATED BY YOU" : "YOUR FIELD NOTES"}</div><h1>{view === "apps" ? "发现应用，从了解开始。" : view === "collections" ? "值得留下的，自成一页。" : "我的使用手记"}</h1><p>{view === "apps" ? "查看应用的用途和官方来源，再读社区里的具体体验。" : view === "collections" ? "收藏零散的发现，整理成有自己取舍的精选页。" : "回到自己走过的步骤，也给后来者留一份参考。"}</p></div>{view === "collections" ? <button className="button primary" disabled={!!opening} onClick={() => void openCollection(null)}>{opening === "new-collection" ? <Busy label="正在准备" /> : <><Plus size={17} />创建精选页</>}</button> : view === "posts" ? <button className="button primary" onClick={() => newPost()}><Pencil size={16} />写新体验</button> : <span className="catalog-count">{session.stats.apps} <small>个应用，持续探索</small></span>}</section>}

      <main className={`community-layout ${view === "collections" ? "collection-layout" : ""}`}>
        <aside className="left-rail"><div className="rail-label">探索方向</div><nav className="category-nav" aria-label="应用分类">{categories.map(item => <button key={item.id} className={category === item.id && ["apps", "discover"].includes(view) ? "active" : ""} onClick={() => { setCategory(item.id); if (!["discover", "apps"].includes(view)) setView("discover"); }}><item.Icon size={17} />{item.label}{category === item.id && ["apps", "discover"].includes(view) && <span />}</button>)}</nav><div className="rail-divider" /><nav className="personal-nav" aria-label="个人内容"><button className={tab === "saved" && view === "discover" ? "selected" : ""} onClick={() => { setTab("saved"); setView("discover"); }}><Bookmark size={17} />我的收藏</button><button className={view === "posts" ? "selected" : ""} onClick={() => navigate("posts")}><FileText size={17} />我的体验</button><button className={view === "collections" ? "selected" : ""} onClick={() => navigate("collections")}><FolderHeart size={17} />我的精选</button><button onClick={() => setSettings(true)}><SlidersHorizontal size={17} />阅读偏好</button></nav><div className="rail-footnote"><span className="hand-line" /><p>使用过的细节，<br />就是有价值的经验。</p><button onClick={() => setHelp(true)}>关于这个社区<ArrowUpRight size={12} /></button></div></aside>

        <section className="main-content" aria-label={view === "discover" ? "社区内容" : view === "apps" ? "应用目录" : view === "posts" ? "我的体验" : "我的精选"}>
          {view === "discover" && <div className="feed-tabs" role="tablist" aria-label="内容排序">{tabs.map(item => <button key={item.id} role="tab" aria-selected={tab === item.id} className={tab === item.id ? "active" : ""} onClick={() => setTab(item.id)}><item.Icon size={16} />{item.title}</button>)}</div>}
          {query && ["discover", "apps"].includes(view) && <div className="search-summary"><Search size={15} />搜索“{query}”<button onClick={() => { setQuery(""); setSearch(""); }}><X size={14} />清除</button></div>}
          {view === "discover" && tab === "verified" && <div className="feed-explanation"><Link2 size={16} /><p>仅收录已核对链上交互的体验。凭据说明交互发生过，不是应用安全或内容质量的保证。</p></div>}
          {view === "discover" && tab === "for-you" && !query && <div className="feed-intro"><span>由你的兴趣与阅读反馈，慢慢找到合拍的内容</span><button onClick={() => setSettings(true)}><ListFilter size={14} />调整偏好</button></div>}
          {loading ? <div className="feed-loading"><Busy label={view === "collections" ? "正在读取精选页" : "正在寻找好内容"} /><div className="skeleton-lines"><span /><span /><span /></div></div> : loadedKey === requestKey && feedError ? <div className="load-error"><Notice error>{feedError}</Notice><button className="button secondary" onClick={() => setReload(value => value + 1)}>重试加载</button></div> : view === "apps" ? <div className="app-catalog">{filteredApps.length ? filteredApps.map(app => <article key={app.id} className="catalog-item"><div className="catalog-item-header"><AppMark app={app} large /><span className="category-label">{categoryName(app.category)}</span></div><h2>{app.name}</h2><p>{app.summary}</p><div className="post-tags">{app.tags.map(tag => <span key={tag}>#{tag}</span>)}</div><div className="catalog-item-actions"><button className="text-link" onClick={() => { setQuery(app.name); setSearch(app.name); setView("discover"); }}>看相关内容<ArrowRight size={14} /></button><ExternalLink href={app.url}>官网</ExternalLink></div><div className="catalog-source"><ExternalLink href={app.source_url}>资料来源</ExternalLink><button onClick={() => newPost(app.id)}><Pencil size={13} />我用过</button></div></article>) : <Empty title="还没找到匹配的应用">试试其他关键词，或切换到全部应用。</Empty>}</div> : view === "collections" ? <div className="collection-list">{collections.length ? collections.map(collection => <article key={collection.id} className={`collection-card collection-${collection.theme}`}><button className="collection-card-main" onClick={() => void openCollection(collection)} disabled={!!opening}><div className="collection-card-label"><FolderHeart size={16} />个人精选<span>第 {collection.version} 版</span></div><h2>{collection.title}</h2><p>{collection.description || "一份由自己整理的应用与阅读清单。"}</p><div className="collection-card-marks">{collection.app_ids.slice(0, 5).map(id => <AppMark key={id} app={appMap.get(id)} />)}<span>{collection.app_ids.length} 个应用 · {collection.post_ids.length} 篇内容</span></div><div className="collection-card-bottom"><span>{dateLabel(collection.updated_at)} 更新</span>{opening === collection.id ? <Busy label="读取中" /> : <span>编辑与预览<ArrowUpRight size={16} /></span>}</div></button></article>) : <Empty title="给喜欢的发现，留个好位置" action={<button className="button primary" disabled={!!opening} onClick={() => void openCollection(null)}><Plus size={16} />创建第一份精选</button>}>挑选应用和阅读内容，排成自己的清单。<br />无需连接钱包，就可以开始整理。</Empty>}</div> : activePosts.length ? <><div className="post-list">{activePosts.map(post => <PostCard key={post.id} post={post} app={appMap.get(post.app_id)} onOpen={value => void openPost(value)} onBookmark={value => void togglePost(value, "bookmark")} onHelpful={value => void togglePost(value, "helpful")} onHide={value => void hidePost(value)} pending={pending} />)}</div>{view === "discover" && feed.next_cursor && <button className="load-more" disabled={loadMore} onClick={() => void nextPage()}>{loadMore ? <Busy label="正在读取" /> : <>再读一些<ArrowDown size={15} /></>}</button>}<div className="feed-end"><span />好内容，慢慢读<span /></div></> : <Empty title={query ? "这里还没有匹配的内容" : view === "posts" ? "第一篇体验，等你落笔" : tab === "saved" ? "喜欢的内容，先收下来" : tab === "verified" ? "还没有带凭据的体验" : "这一页，等一份真实体验"} action={query ? <button className="button secondary" onClick={() => { setQuery(""); setSearch(""); setCategory("all"); }}>查看全部内容</button> : tab === "saved" ? <button className="button secondary" onClick={() => setTab("for-you")}>去发现内容<ArrowRight size={15} /></button> : <button className="button primary" onClick={() => newPost()}><Pencil size={15} />写下我的体验</button>}>{query ? "换一个关键词，或者看看其他应用分类。" : tab === "saved" ? "在内容旁点击收藏，之后就能在这里找到。" : tab === "verified" ? "发布使用体验后，可附上当前钱包的交易哈希进行核对。" : "分享具体的场景、步骤和感受，给下一位使用者留一点线索。"}</Empty>}
        </section>

        {view !== "collections" && <aside className="right-rail"><section className="rail-discovery"><div className="section-title"><h2>认识一个应用</h2><button aria-label="查看全部应用" onClick={() => navigate("apps")}><ArrowUpRight size={16} /></button></div>{apps.slice(0, 3).map(app => <button className="small-app" key={app.id} onClick={() => { setSearch(app.name); setQuery(app.name); setCategory("all"); setView("discover"); }}><AppMark app={app} /><span><strong>{app.name}</strong><small>{categoryName(app.category)}</small></span><ChevronRight size={14} /></button>)}<button className="rail-text-link" onClick={() => navigate("apps")}>打开应用目录<ArrowRight size={14} /></button></section><section className="proof-explainer"><div className="proof-stamp"><Link2 size={25} strokeWidth={1.5} /></div><span className="eyebrow">A LITTLE MORE CONTEXT</span><h2>一个凭据，<br />多一份上下文。</h2><p>交易记录能帮助核对一次链上交互。体验好坏，仍然留给具体内容和你的判断。</p><button onClick={() => setHelp(true)}>交互凭据说明<ArrowUpRight size={14} /></button></section><section className="community-note"><h3><HeartHandshake size={18} />社区，从真实开始</h3><p>已有 <strong>{session.stats.reviews}</strong> 篇使用体验，其中 <strong>{session.stats.verified}</strong> 篇附有已核对的交互凭据。</p>{session.stats.reviews === 0 && <small>当前内容中的资料笔记均带来源标注。欢迎留下第一份亲身体验。</small>}</section><div className="rail-footer"><button onClick={() => setHelp(true)}>关于用过再说</button><span>少一点噪声，多一点细节。</span><small>UsedIt · 用过再说</small></div></aside>}
      </main>
      <footer className="site-footer"><Logo /><p>亲自用过的，每一处细节都值得说。</p><button onClick={() => setHelp(true)}>关于社区<ArrowUpRight size={14} /></button></footer>
    </>}

    {opening && !collectionEditor && <div className="opening-indicator" role="status"><Busy label="正在读取内容" /></div>}
    {detail && <PostDetail key={detail.id} post={detail} app={appMap.get(detail.app_id)} onClose={() => setDetail(null)} onBookmark={post => void togglePost(post, "bookmark")} onHelpful={post => void togglePost(post, "helpful")} onEdit={editPost} onDelete={deletePost} onUpdate={patchPost} pending={pending} />}
    {postEditor && session && <PostEditor apps={apps} user={session.user} draft={editingPost ? editDraft : draft} onDraft={editingPost ? setEditDraft : setDraft} initial={editingPost} onClose={() => setPostEditor(false)} onSaved={savePost} connectWallet={connectWallet} />}
    {settings && session && <Settings user={session.user} apps={apps} onUser={updateUser} onClose={() => setSettings(false)} onLogout={logout} connectWallet={connectWallet} />}
    {collectionEditor && session && <CollectionEditor initial={editingCollection} apps={apps} posts={collectionPosts} freeform={session.capabilities.freeform} onSaved={saveCollection} onClose={() => setCollectionEditor(false)} />}
    {help && <Modal title="关于用过再说" onClose={() => setHelp(false)}><div className="about-content"><div className="eyebrow">EXPERIENCE, WITH CONTEXT.</div><h2>先看清楚，<br />再做自己的选择。</h2><p>用过再说是一个分享链上应用使用体验的社区。欢迎聊具体步骤、遇到的问题，以及值得留下的小发现。</p><section><h3><Pencil size={17} />使用体验与资料笔记</h3><p>使用体验由连接钱包的用户发布。资料笔记根据官方资料整理，始终附带来源标识，不计入使用体验数量。</p></section><section><h3><Link2 size={17} />交互凭据说明什么？</h3><p>核对范围包括 Ethereum 主网的交易发送者、目标合约、执行结果和确认深度。凭据仅表明这次交互发生过，不是对人的身份、应用安全或内容质量的认证。</p></section><section><h3><SlidersHorizontal size={17} />推荐如何参考我的反馈？</h3><p>阅读偏好、收藏、有用标记和不感兴趣反馈共同帮助整理内容顺序。你随时可以查看最新分享，或修改阅读偏好。</p></section><section><h3><Wallet size={17} />什么时候需要钱包？</h3><p>发布体验时需要签名登录。浏览、收藏和创建精选页无需连接钱包。签名登录不要求转账；关闭钱包操作后，正在编辑的体验会留在本次页面中。</p></section></div></Modal>}
    {toast && <Toast message={toast.text} action={toast.action} onClose={() => setToast(null)} />}
  </div>;
}
