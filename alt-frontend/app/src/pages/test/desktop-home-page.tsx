import { DesktopHomePage } from '../../components/desktop/home/DesktopHomePage';
import { ThemeProvider } from '../../providers/ThemeProvider';
import { ChakraProvider } from '@chakra-ui/react';

export default function DesktopHomePageTest() {
  return (
    <ChakraProvider>
      <ThemeProvider>
        <DesktopHomePage />
      </ThemeProvider>
    </ChakraProvider>
  );
}