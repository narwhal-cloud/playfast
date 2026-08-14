import './App.css'
import {Switch, Version, ProxyList} from "../wailsjs/go/main/App";
import {h} from 'preact';
import {Announcement} from "./component/Announcement";
import {useLayoutEffect, useState, useEffect, useRef} from "preact/compat";
// 为particlesJS添加类型声明
declare global {
    interface Window {
        particlesJS: any;
    }
}

// 粒子背景配置
const particlesConfig = {
    particles: {
        number: { value: 80, density: { enable: true, value_area: 800 } },
        color: { value: '#8a2be2' },
        shape: { type: 'circle' },
        opacity: { value: 0.5, random: false },
        size: { value: 3, random: true },
        line_linked: {
            enable: true,
            distance: 150,
            color: '#9932cc',
            opacity: 0.3,
            width: 1
        },
        move: {
            enable: true,
            speed: 2,
            direction: 'none',
            random: false,
            straight: false,
            out_mode: 'out',
            bounce: false
        }
    },
    interactivity: {
        detect_on: 'canvas',
        events: {
            onhover: { enable: true, mode: 'grab' },
            onclick: { enable: true, mode: 'push' },
            resize: true
        }
    },
    retina_detect: true
};

// 定义流量数据类型
interface TrafficData {
    up: number;
    down: number;
}

export function App(props: any) {
    const [userUUID, setUserUUID] = useState<string>(() => localStorage.getItem('playfast_uuid') || '');
    const [status, setStatus] = useState('开始加速');
    const [isLoading, setIsLoading] = useState(false);
    const [getVersion, setVersion] = useState("v1.0.0");
    const [getRegion, setRegion] = useState(''); // 初始为空，等待服务器列表加载后设置
    const [proxyList, setProxyList] = useState<string[]>([]); // 新增：服务器列表状态
    const [isAccelerated, setIsAccelerated] = useState(false);
    const [isHostMode, setIsHostMode] = useState(false); // 新增主机模式状态
    const [stats, setStats] = useState({download: 0, upload: 0, totalTraffic:0, uptime: 0});
    const timerRef = useRef<number | null>(null);

    // 重新加载服务器列表
    const loadProxies = async (uuid: string) => {
        try {
            const servers = await ProxyList(uuid);
            setProxyList(servers);
            if (servers.length > 0) {
                setRegion(servers[0]);
            } else {
                setRegion('');
            }
        } catch (error) {
            console.error("获取服务器列表失败:", error);
        }
    };

    // 处理 UUID 变化
    const handleUUIDChange = (e: any) => {
        const val = e.target.value;
        setUserUUID(val);
        localStorage.setItem('playfast_uuid', val);
        loadProxies(val);
    };
    // WebSocket连接引用
    const wsRef = useRef<WebSocket | null>(null);
    // 格式化字节数为人类可读格式
    const formatBytes = (bytes: number) => {
        if (bytes === 0) return '0 B';
        const units = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(1024));
        return (bytes / Math.pow(1024, i)).toFixed(1) + ' ' + units[i];
    };
    // 格式化速度为人类可读格式（B/s, KB/s, MB/s, GB/s）
    const formatSpeed = (bytesPerSecond: number) => {
        if (bytesPerSecond === 0) return '0 B/s';
        const units = ['B/s', 'KB/s', 'MB/s', 'GB/s'];
        const i = Math.floor(Math.log(bytesPerSecond) / Math.log(1024));
        return (bytesPerSecond / Math.pow(1024, i)).toFixed(1) + ' ' + units[Math.min(i, units.length - 1)];
    };
    // 更新状态并处理状态切换逻辑
    const updateStatus = () => {
        const newStatus = status === "开始加速" ? "停止加速" : "开始加速";
        setStatus(newStatus);
        setIsAccelerated(newStatus === "停止加速");
        if (newStatus === "停止加速") {
            // 先清除可能存在的旧计时器
            if (timerRef.current !== null) {
                clearInterval(timerRef.current);
                timerRef.current = null;
            }
            // 连接WebSocket获取实时流量数据
            connectWebSocket();
            setStats({download: 0, upload: 0, totalTraffic: 0, uptime: 0});
            // 启动计时器，每秒更新一次uptime
            timerRef.current = window.setInterval(() => {
                setStats(prev => {
                    const currentUptime = prev.uptime;
                    return {
                        ...prev,
                        uptime: currentUptime + 1/60  // 每秒增加1/60分钟
                    };
                });
            }, 1000);
        } else {
            // 断开WebSocket连接
            disconnectWebSocket();
            // 清除计时器
            if (timerRef.current !== null) {
                clearInterval(timerRef.current);
                timerRef.current = null;
            }
            setStats({download: 0, upload: 0, totalTraffic: 0, uptime: 0});
        }
    };
    // 连接WebSocket
    const connectWebSocket = () => {
        // 关闭已有连接
        disconnectWebSocket();
        const ws = new WebSocket('ws://127.0.0.1:54713/traffic');
        ws.onopen = () => {
            console.log('WebSocket连接已建立');
        };
        ws.onmessage = (event) => {
            try {
                const data: TrafficData = JSON.parse(event.data);
                // 使用函数式更新来累加总流量，确保基于最新状态
                const trafficSum = data.up + data.down;
                // 在总流量更新的同时更新stats中的显示，但保留uptime值
                setStats(prev => ({
                    ...prev,
                    download: data.down,
                    upload: data.up,
                    totalTraffic: prev.totalTraffic + trafficSum
                    // 不更新uptime，让timer专门处理
                }));
            } catch (error) {
                console.error('解析WebSocket数据失败:', error);
            }
        };
        ws.onerror = (error) => {
            console.error('WebSocket错误:', error);
        };
        ws.onclose = () => {
            console.log('WebSocket连接已关闭');
        };
        wsRef.current = ws;
    };
    // 断开WebSocket连接
    const disconnectWebSocket = () => {
        if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
            wsRef.current.close();
            wsRef.current = null;
        }
    };
    // 组件卸载时断开WebSocket连接
    useEffect(() => {
        return () => {
            if (timerRef.current !== null) {
                clearInterval(timerRef.current);
                timerRef.current = null;
            }
            disconnectWebSocket();
        };
    }, []);
    // 启动和停止加速
    function sw() {
        setIsLoading(true); // 点击时将 loading 状态设为 true
        Switch(userUUID, status === "开始加速", getRegion, isHostMode).then(
            function (res) {
                setIsLoading(false);
                if (res==""){
                    updateStatus();
                }
            }
        );
    }
    // 初始化获取版本信息和服务器列表
    useLayoutEffect(() => {
        async function fetchInitialData() {
            try {
                // 获取版本信息
                const version = await Version();
                setVersion(version);

                // 获取服务器列表
                await loadProxies(userUUID);
            } catch (error) {
                console.error("获取初始数据失败:", error);
            }
        }
        fetchInitialData().then(_=> {});
    }, []);
    // 初始化粒子背景
    useEffect(() => {
        // 检查window和particlesJS是否可用
        if (typeof window !== 'undefined' && window.particlesJS) {
            window.particlesJS('particles-background', particlesConfig);
        } else {
            // 如果particlesJS不可用，动态加载脚本
            const script = document.createElement('script');
            script.src = 'https://cdn.jsdelivr.net/npm/particlesjs@2.2.3/dist/particles.min.js';
            script.async = true;
            script.onload = () => {
                window.particlesJS('particles-background', particlesConfig);
            };
            document.body.appendChild(script);
        }
    }, []);
    function onChange(e: any) {
        setRegion(e.target.value);
    }
    // 主机模式开关处理函数
    function handleHostModeChange(e: any) {
        setIsHostMode(e.target.checked);
    }
    return (
        <div>
            <div id="App">
                {/* 粒子背景 */}
                <div id="particles-background"></div>
                
                {/* Logo区域 */}
                <div className="logo-area">
                    <h1><span className="highlight">Play</span>Fast</h1>
                </div>
                
                {/* 公告组件 */}
                <Announcement/>
                
                {/* 右侧控制面板 */}
                <div id="RightBox">
                    <h2>加速控制面板</h2>
                    
                    {/* 状态指示器 */}
                    <div className="status-indicator">
                        <div className={`indicator-dot ${isAccelerated ? 'active' : ''}`}></div>
                        <div className="indicator-text">
                            {isAccelerated ? '加速中' : '未加速'}
                        </div>
                    </div>
                    
                    {/* 网络状态显示 */}
                  
                        {isAccelerated && (
                        <div className="network-stats">
                            <div className="stats-grid">
                                <div className="stat-item">
                                    <span className="stat-label">下载速度</span>
                                    <span className="stat-value">{formatSpeed(stats.download)}</span>
                                </div>
                                <div className="stat-item">
                                    <span className="stat-label">上传速度</span>
                                    <span className="stat-value">{formatSpeed(stats.upload)}</span>
                                </div>
                                <div className="stat-item">
                                    <span className="stat-label">流量总计</span>
                                    <span className="stat-value">{formatBytes(stats.totalTraffic)}</span>
                                </div>
                                <div className="stat-item">
                                    <span className="stat-label">加速时长</span>
                                    <span className="stat-value">{Math.floor(stats.uptime)}分钟{Math.floor((stats.uptime % 1) * 60)}秒</span>
                                </div>
                            </div>
                        </div>
                        )}
             
                    
                    {/* 区域选择和加速按钮 */}
                    <div id="Bottom">
                        <div>
                            <label htmlFor="user-uuid-input">用户凭证：</label>
                            <input
                                id="user-uuid-input"
                                type="text"
                                placeholder="请输入用户 UUID"
                                value={userUUID}
                                onChange={handleUUIDChange}
                                disabled={isAccelerated}
                                style={{
                                    width: '100%',
                                    padding: '6px 10px',
                                    borderRadius: '6px',
                                    border: '1px solid rgba(138, 43, 226, 0.4)',
                                    background: 'rgba(18, 18, 18, 0.6)',
                                    color: '#fff',
                                    outline: 'none',
                                    marginBottom: '8px',
                                    fontSize: '0.85rem'
                                }}
                            />
                        </div>
                        <div>
                            <label htmlFor="region-select">加速节点：</label>
                            <select id="region-select" value={getRegion} onChange={onChange} disabled={isAccelerated}>
                                {proxyList.map((server, index) => (
                                    <option key={index} value={server}>{server}</option>
                                ))}
                            </select>
                        </div>

                        {!isAccelerated && (
                            <div className="host-mode-toggle">
                                <label htmlFor="host-mode">
                                    <input
                                        type="checkbox"
                                        id="host-mode"
                                        checked={isHostMode}
                                        onChange={handleHostModeChange}
                                    />
                                    主机模式
                                </label>
                            </div>
                        )}

                        <button 
                            className={`btn ${isAccelerated ? 'accelerated' : ''} ${isLoading ? 'loading' : ''}`} 
                            onClick={sw} 
                            disabled={isLoading}
                        >
                            {!isLoading && status}
                        </button>
                    </div>
                </div>
                {/* 版本信息 */}
                <div className="version-info">
                    PlayFast 加速器 {getVersion} | 享受极致游戏体验
                </div>
            </div>
        </div>
    );
}
