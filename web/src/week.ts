import type {Entry,Practice} from './store.ts';
export const practiceColors:Record<Practice,string>={prostrations:'#efd18a','diamond-mind':'#e9b06d',mandala:'#efc3a4','guru-yoga':'#d99c77'};
export function weeklyPractice(entries:Entry[],dates:Date[],timeZone:string){
 const key=(date:Date)=>new Intl.DateTimeFormat('en-CA',{timeZone,year:'numeric',month:'2-digit',day:'2-digit'}).format(date);
 return dates.map(date=>{
  const dayKey=key(date);const counts:Record<Practice,number>={prostrations:0,'diamond-mind':0,mandala:0,'guru-yoga':0};
  for(const entry of entries){if(!entry.initial&&key(new Date(entry.occurredAt))===dayKey)counts[entry.practice]+=entry.total;}
  return {key:dayKey,day:new Intl.DateTimeFormat('en-US',{weekday:'short',timeZone}).format(date),counts,total:Object.values(counts).reduce((sum,n)=>sum+n,0)};
 });
}
