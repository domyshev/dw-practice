import {createContext,useContext,useEffect,useMemo,useState,type ReactNode} from 'react';
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
 useEffect(()=>{document.documentElement.dataset.theme=mode;document.documentElement.style.colorScheme=mode;document.querySelector('meta[name="theme-color"]')?.setAttribute('content',mode==='dark'?'#18201d':'#f6f7f3')},[mode]);
 const theme=useMemo(()=>createTheme({palette:{mode,primary:{main:mode==='dark'?'#a3c6b2':'#28594d'},secondary:{main:'#b78d4b'},background:{default:mode==='dark'?'#18201d':'#f6f7f3',paper:mode==='dark'?'#222c27':'#ffffff'},text:{primary:mode==='dark'?'#e4ece6':'#243a34',secondary:mode==='dark'?'#a6b6aa':'#78847c'}},typography:{fontFamily:'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',button:{textTransform:'none',fontWeight:600}},shape:{borderRadius:12},components:{MuiButton:{styleOverrides:{root:{boxShadow:'none',borderRadius:10,padding:'10px 18px',minHeight:44}}},MuiIconButton:{styleOverrides:{root:{minWidth:44,minHeight:44}}},MuiTextField:{defaultProps:{size:'small'}},MuiDialog:{styleOverrides:{paper:{borderRadius:22}}}}}),[mode]);
 function choose(value:ThemePreference){setPreference(value);try{localStorage.setItem('dw-theme',value)}catch{}}
 return <AppearanceContext.Provider value={{preference,choose}}><ThemeProvider theme={theme}><CssBaseline/>{children}</ThemeProvider></AppearanceContext.Provider>;
}
