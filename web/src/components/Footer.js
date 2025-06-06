import React, { useEffect, useState, useContext } from 'react';
import { useTranslation } from 'react-i18next';
import { getFooterHTML, getSystemName } from '../helpers';
import { Layout, Tooltip } from '@douyinfe/semi-ui';
import { StyleContext } from '../context/Style/index.js';

const FooterBar = () => {
  const { t } = useTranslation();
  const systemName = getSystemName();
  const [footer, setFooter] = useState(getFooterHTML());
  const [styleState] = useContext(StyleContext);
  let remainCheckTimes = 5;

  const loadFooter = () => {
    let footer_html = localStorage.getItem('footer_html');
    if (footer_html) {
      setFooter(footer_html);
    }
  };

  const defaultFooter = (
    <div className='custom-footer'>
      便携AI（bianxie.ai），让您更便携的使用AI。
    </div>
  );

  useEffect(() => {
    const timer = setInterval(() => {
      if (remainCheckTimes <= 0) {
        clearInterval(timer);
        return;
      }
      remainCheckTimes--;
      loadFooter();
    }, 200);
    return () => clearTimeout(timer);
  }, []);

  useEffect(() => {
        // 创建并加载脚本
        const script = document.createElement('script');
        script.charset = 'UTF-8';
        script.id = 'LA_COLLECT';
        script.src = '//sdk.51.la/js-sdk-pro.min.js';
        // script.async = true;
        document.head.appendChild(script);

        // 在脚本加载完成后初始化 51.la
        script.onload = () => {
            if (window.LA) {
                window.LA.init({ id: '3IwNYYQ2DYFgLCyv', ck: '3IwNYYQ2DYFgLCyv' });
            }
        };

        // 清理函数，在组件卸载时移除脚本
        return () => {
            document.head.removeChild(script);
        };
    }, []); // 空依赖数组，表示这个 effect 只会在组件挂载和卸载时执行

  return (
    <div
      style={{
        textAlign: 'center',
        paddingBottom: '5px',
      }}
    >
      {footer ? (
        <div
          className='custom-footer'
          dangerouslySetInnerHTML={{ __html: footer }}
        ></div>
      ) : (
        defaultFooter
      )}
    </div>
  );
};

export default FooterBar;
