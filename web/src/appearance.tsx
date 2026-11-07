import {createContext,useContext,useEffect,useMemo,useState,type ReactNode} from 'react';
import {applyNativeTheme} from './native';
import {ThemeProvider,createTheme,CssBaseline} from '@mui/material';
export type ThemePreference='light'|'system'|'dark';
const AppearanceContext=createContext<{preference:ThemePreference;choose:(value:ThemePreference)=>void}>({preference:'system',choose:()=>{}});
export const useAppearance=()=>useContext(AppearanceContext);
function savedPreference():ThemePreference{
 try{const value=localStorage.getItem('dw-theme');if(value==='light'||value==='dark'||value==='system')return value}catch{}
 return 'system';
}
export function AppearanceProvider({children}:{children:ReactNode}){
 const [preference,setPreference]=useState<ThemePreference>(savedPreference);
 const [systemDark,setSystemDark]=useState(()=>window.matchMedia('(prefers-color-scheme: dark)').matches);
 useEffect(()=>{const media=window.matchMedia('(prefers-color-scheme: dark)');const changed=()=>setSystemDark(media.matches);media.addEventListener('change',changed);return()=>media.removeEventListener('change',changed)},[]);
 const mode=preference==='system'?(systemDark?'dark':'light'):preference;
 useEffect(()=>{document.documentElement.dataset.theme=mode;document.documentElement.style.colorScheme=mode;document.querySelector('meta[name="theme-color"]')?.setAttribute('content',mode==='dark'?'#211917':'#faf5ee')},[mode]);
 useEffect(()=>{void applyNativeTheme(mode).catch(error=>console.error('Unable to update the status bar',error))},[mode]);
 const theme=useMemo(()=>createTheme({palette:{mode,primary:{main:mode==='dark'?'#e6b88a':'#96533d'},secondary:{main:'#b78d4b'},background:{default:mode==='dark'?'#211917':'#faf5ee',paper:mode==='dark'?'#30231f':'#fffaf5'},text:{primary:mode==='dark'?'#f3e5d8':'#4b3028',secondary:mode==='dark'?'#c9ac99':'#906e5e'}},typography:{fontFamily:'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',button:{textTransform:'none',fontWeight:600}},shape:{borderRadius:12},components:{MuiTooltip:{defaultProps:{arrow:true},styleOverrides:{tooltip:{backgroundColor:mode==='dark'?'#3c2b25':'#fffaf5',color:mode==='dark'?'#f3e5d8':'#4b3028',border:`1px solid ${mode==='dark'?'#765445':'#e8d7c8'}`,borderRadius:4,padding:'6px 9px',fontSize:11,lineHeight:1.5,boxShadow:mode==='dark'?'0 3px 12px #00000040':'0 3px 12px #4b30281a'},arrow:{color:mode==='dark'?'#3c2b25':'#fffaf5','&::before':{border:`1px solid ${mode==='dark'?'#765445':'#e8d7c8'}`}}}},MuiButton:{styleOverrides:{root:{boxShadow:'none',borderRadius:10,padding:'7px 14px',minHeight:38,fontSize:13}}},MuiIconButton:{styleOverrides:{root:{minWidth:36,minHeight:36,padding:6}}},MuiTextField:{defaultProps:{size:'small'}},MuiDialog:{styleOverrides:{paper:{borderRadius:22}}}}}),[mode]);
 function choose(value:ThemePreference){setPreference(value);try{localStorage.setItem('dw-theme',value)}catch{}}
 return <AppearanceContext.Provider value={{preference,choose}}><ThemeProvider theme={theme}><CssBaseline/>{children}</ThemeProvider></AppearanceContext.Provider>;
}
