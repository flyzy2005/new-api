import React, { useEffect, useState } from 'react';

import { getFooterHTML, getSystemName } from '../helpers';
import { Layout, Tooltip } from '@douyinfe/semi-ui';

const Footer = () => {
  const systemName = getSystemName();
  const [footer, setFooter] = useState(getFooterHTML());
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

  return (
    <Layout>
      <Layout.Content style={{ textAlign: 'center' }}>
        {footer ? (
          <Tooltip content={defaultFooter}>
            <div
              className='custom-footer'
              dangerouslySetInnerHTML={{ __html: footer }}
            ></div>
          </Tooltip>
        ) : (
          defaultFooter
        )}
      </Layout.Content>
    </Layout>
  );
};

export default Footer;
