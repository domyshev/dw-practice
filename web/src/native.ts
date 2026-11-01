import {Capacitor} from '@capacitor/core';
export const isNative=()=>Capacitor.isNativePlatform();
export async function applyNativeTheme(mode:'light'|'dark'){
 if(!isNative())return;
 const {StatusBar,Style}=await import('@capacitor/status-bar');
 await StatusBar.setStyle({style:mode==='dark'?Style.Dark:Style.Light});
}
export function initializeNative(){
 if(!isNative())return;
 document.documentElement.dataset.platform=Capacitor.getPlatform();
 // Keep external documentation in Safari View Controller, outside the app's authenticated WebView.
 document.addEventListener('click',event=>{
  const link=(event.target as Element)?.closest?.('a[href]');if(!link)return;
  const url=new URL((link as HTMLAnchorElement).href);if(!['https:','http:'].includes(url.protocol))return;
  event.preventDefault();void import('@capacitor/browser').then(({Browser})=>Browser.open({url:url.href})).catch(error=>console.error('Unable to open external link',error));
 });
}
export async function shareExport(filename:string,payload:string|Uint8Array){
 const [{Filesystem,Directory},{Share}]=await Promise.all([import('@capacitor/filesystem'),import('@capacitor/share')]);
 const bytes=typeof payload==='string'?new TextEncoder().encode(payload):payload;
 let binary='';for(let offset=0;offset<bytes.length;offset+=8192)binary+=String.fromCharCode(...bytes.subarray(offset,offset+8192));
 const file=await Filesystem.writeFile({path:`exports/${filename}`,data:btoa(binary),directory:Directory.Cache,recursive:true});
 try{await Share.share({title:'dw practice history',files:[file.uri],dialogTitle:'Export practice history'})}catch(error){if(!/cancel/i.test((error as Error).message))throw error}
}
