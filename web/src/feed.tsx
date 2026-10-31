import {useCallback,useEffect,useRef,useState,type ReactNode} from 'react';
import {useVirtualizer} from '@tanstack/react-virtual';
import {Alert,Button,CircularProgress,IconButton,Menu,MenuItem,Tooltip} from '@mui/material';
import {DateRangeOutlined} from '@mui/icons-material';
import {api,useAppSelector,type Entry,type Practice} from './store';
export const feedPeriods=[{id:1,label:'Last month'},{id:3,label:'Last 3 months'},{id:6,label:'Last 6 months'},{id:12,label:'Last year'},{id:0,label:'All time'}];
export function periodStart(months:number,now=new Date()){
 if(!months)return '';
 const date=new Date(now);const day=date.getDate();date.setDate(1);date.setMonth(date.getMonth()-months);
 const end=new Date(date.getFullYear(),date.getMonth()+1,0).getDate();date.setDate(Math.min(day,end));return date.toISOString();
}
type Page={items:Entry[];nextCursor:string};
export function PracticeFeed({practice,title,renderEntry}:{practice?:Practice;title:string;renderEntry:(entry:Entry)=>ReactNode}){
 const revision=useAppSelector(s=>s.revision),generation=useAppSelector(s=>s.sessionGeneration);
 const [months,setMonths]=useState(1),[items,setItems]=useState<Entry[]>([]),[cursor,setCursor]=useState(''),[more,setMore]=useState(true),[busy,setBusy]=useState(true),[error,setError]=useState('');
 const parent=useRef<HTMLDivElement>(null),sequence=useRef(0),inflight=useRef(false);
 const [anchor,setAnchor]=useState<HTMLElement|null>(null);
 const [since,setSince]=useState(()=>periodStart(1));
 const fetchPage=useCallback(async(next:string,replace=false)=>{
  if(inflight.current)return;inflight.current=true;const request=sequence.current;setBusy(true);setError('');
  const params=new URLSearchParams();if(since)params.set('since',since);if(practice)params.set('practice',practice);if(next)params.set('cursor',next);
  try{const page=await api<Page>(`/entries/page?${params}`);if(request!==sequence.current)return;setItems(old=>replace?page.items:[...old,...page.items.filter(e=>!old.some(item=>item.id===e.id))]);setCursor(page.nextCursor);setMore(!!page.nextCursor)}catch(e){if(request===sequence.current)setError((e as Error).message)}finally{if(request===sequence.current){inflight.current=false;setBusy(false)}}
 },[since,practice]);
 useEffect(()=>{sequence.current++;inflight.current=false;setItems([]);setCursor('');setMore(true);parent.current?.scrollTo(0,0);void fetchPage('',true);return()=>{sequence.current++;inflight.current=false}},[fetchPage,revision,generation]);
 const virtual=useVirtualizer({count:items.length+(more?1:0),getScrollElement:()=>parent.current,estimateSize:()=>180,overscan:3,paddingEnd:16,getItemKey:index=>items[index]?.id||'load-more'});
 const rows=virtual.getVirtualItems(),last=rows.at(-1)?.index??-1;
 useEffect(()=>{if(last>=items.length-3&&more&&!busy&&!error)void fetchPage(cursor)},[last,items.length,more,busy,error,cursor,fetchPage]);
 return <><div className="section-heading feed-heading"><h2>{title}</h2><div className="feed-period-choice"><span>{feedPeriods.find(period=>period.id===months)!.label}</span><Tooltip title="Change period"><IconButton className="period-button" aria-label={`${title}: change period`} aria-haspopup="menu" aria-expanded={!!anchor} onClick={event=>setAnchor(event.currentTarget)}><DateRangeOutlined/></IconButton></Tooltip></div></div><Menu anchorEl={anchor} open={!!anchor} onClose={()=>setAnchor(null)}>{feedPeriods.map(period=><MenuItem key={period.id} selected={months===period.id} onClick={()=>{setMonths(period.id);setSince(periodStart(period.id));setAnchor(null)}}>{period.label}</MenuItem>)}</Menu>

 <div ref={parent} className="virtual-feed" role="region" aria-label={`${title} entries`} tabIndex={0} aria-busy={busy}>
 {!items.length&&!busy&&!error?<div className="empty-state"><h3>No entries in this period</h3></div>:<div style={{height:virtual.getTotalSize(),width:'100%',position:'relative'}}>{rows.map(row=><div key={row.key} data-index={row.index} ref={virtual.measureElement} className="virtual-entry" style={{position:'absolute',top:0,left:0,width:'100%',transform:`translateY(${row.start}px)`}}>{row.index<items.length?renderEntry(items[row.index]):<div className="feed-loading">{busy&&<CircularProgress size={22}/>}</div>}</div>)}</div>}
 {error&&<Alert severity="error" action={<Button onClick={()=>void fetchPage(cursor,!items.length)}>Retry</Button>}>{error}</Alert>}
 </div></>;
}
