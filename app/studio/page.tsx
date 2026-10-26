export const metadata={title:'Orbit Studio · 从想法到应用',description:'描述需求，运行预览，连续修改并保存每一个应用版本。'};
import Studio from '../ui/studio/Studio';
// 创作工作台复用 Orbit 身份与服务端数据，与社区保持独立入口。
export default function StudioPage(){return <Studio/>}
