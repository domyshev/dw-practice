import type {Entry,Practice} from './store.ts';
export const PRACTICE_GOAL=111111;
export const chartPeriods=[
 {id:'week',title:'A week of practice'},
 {id:'month',title:'A month of practice'},
 {id:'quarter',title:'A quarter of practice'},
 {id:'year',title:'A year of practice'},
 {id:'last-year',title:'Last year of practice'}
] as const;
export type ChartPeriod=typeof chartPeriods[number]['id'];
export function parseChartPeriod(value:unknown):ChartPeriod{return chartPeriods.some(p=>p.id===value)?value as ChartPeriod:'week'}
const emptyCounts=():Record<Practice,number>=>({prostrations:0,'diamond-mind':0,mandala:0,'guru-yoga':0});
const dayMs=86400000;
const key=(serial:number)=>new Date(serial*dayMs).toISOString().slice(0,10);
const serial=(year:number,month:number,day:number)=>Date.UTC(year,month,day)/dayMs;
function localKey(date:Date,timeZone:string){
 const parts=new Intl.DateTimeFormat('en-CA',{timeZone,year:'numeric',month:'2-digit',day:'2-digit'}).formatToParts(date);
 const value=(type:string)=>parts.find(part=>part.type===type)!.value;
 return `${value('year')}-${value('month')}-${value('day')}`;
}
export function practiceActivity(entries:Entry[],period:ChartPeriod,timeZone:string,now=new Date()){
 const todayKey=localKey(now,timeZone);const [year,month,day]=todayKey.split('-').map(Number);const today=serial(year,month-1,day);
 let start=today-6,end=today,unit='Daily totals';
 if(period==='month'){start=serial(year,month-1,1);end=serial(year,month,1)-1;unit='Weekly totals'}
 if(period==='quarter'){const quarter=Math.floor((month-1)/3)*3;start=serial(year,quarter,1);end=serial(year,quarter+3,1)-1;unit='Monthly totals'}
 if(period==='year'||period==='last-year'){const selectedYear=period==='last-year'?year-1:year;start=serial(selectedYear,0,1);end=serial(selectedYear+1,0,1)-1;unit='Monthly totals'}
 const buckets:{key:string;endKey:string;label:string;counts:Record<Practice,number>;total:number}[]=[];
 if(period==='week'||period==='month'){
  const stride=period==='week'?1:7;
  for(let first=start;first<=end;first+=stride){
   const last=Math.min(first+stride-1,end);const date=new Date(first*dayMs);
   const label=period==='week'?new Intl.DateTimeFormat('en-US',{timeZone:'UTC',weekday:'short'}).format(date):first===last?String(date.getUTCDate()):`${date.getUTCDate()}–${new Date(last*dayMs).getUTCDate()}`;
   buckets.push({key:key(first),endKey:key(last),label:period==='week'&&first===today?'Today':label,counts:emptyCounts(),total:0});
  }
 }else{
  for(let first=start;first<=end;){const date=new Date(first*dayMs);const next=serial(date.getUTCFullYear(),date.getUTCMonth()+1,1);buckets.push({key:key(first),endKey:key(Math.min(next-1,end)),label:new Intl.DateTimeFormat('en-US',{timeZone:'UTC',month:'short'}).format(date),counts:emptyCounts(),total:0});first=next}
 }
 const formatter=new Intl.DateTimeFormat('en-CA',{timeZone,year:'numeric',month:'2-digit',day:'2-digit'});
 for(const entry of entries){
  if(entry.initial)continue;
  const parts=formatter.formatToParts(new Date(entry.occurredAt));const part=(type:string)=>parts.find(p=>p.type===type)!.value;const entryKey=`${part('year')}-${part('month')}-${part('day')}`;const bucket=buckets.find(bucket=>entryKey>=bucket.key&&entryKey<=bucket.endKey);
  if(bucket){bucket.counts[entry.practice]+=entry.total;bucket.total+=entry.total}
 }
 const counts=emptyCounts();for(const bucket of buckets)for(const practice of Object.keys(counts) as Practice[])counts[practice]+=bucket.counts[practice];
 const rangeDate=(dateKey:string)=>new Intl.DateTimeFormat('en-GB',{timeZone:'UTC',day:'numeric',month:'short',year:'numeric'}).format(new Date(`${dateKey}T12:00:00Z`));
 return {title:chartPeriods.find(p=>p.id===period)!.title,description:`${unit} · ${rangeDate(key(start))} – ${rangeDate(key(end))}`,buckets,counts,total:Object.values(counts).reduce((sum,n)=>sum+n,0)};
}
